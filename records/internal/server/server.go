package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"records/internal/ai"
	"records/internal/config"
	"records/internal/engine"
	"records/internal/feishu"
	"records/internal/models"
	"records/internal/orchestrator"
	"records/internal/repository"
	"records/internal/worker"
	"records/pkg/logger"

	"github.com/jmoiron/sqlx"
)

// Server 服务器
type Server struct {
	config       *config.Config
	db           *sqlx.DB
	feishuClient feishu.Client
	aiClient     ai.Client
	orchestrator *orchestrator.TurnOrchestrator
	outputWorker *worker.OutputWorker
	logger       logger.Logger
	httpServer   *http.Server
	userLocks    sync.Map // 用户级锁，key: userID, value: *sync.Mutex
}

// apiPrefix 返回 API 路径前缀，已规范化（无尾部斜杠，空则默认 /api）
func (s *Server) apiPrefix() string {
	p := strings.TrimSpace(s.config.Server.APIPrefix)
	if p == "" {
		return "/api"
	}
	return "/" + strings.Trim(p, "/")
}

// webPrefix 返回 Web 页面路径前缀，已规范化（/ 或 /xxx）
func (s *Server) webPrefix() string {
	p := strings.TrimSpace(s.config.Server.WebPrefix)
	if p == "" || p == "/" {
		return "/"
	}
	return "/" + strings.Trim(p, "/")
}

// New 创建新的服务器实例
func New(
	cfg *config.Config,
	db *sqlx.DB,
	feishuClient feishu.Client,
	logger logger.Logger,
) *Server {
	// 初始化AI客户端
	aiClient := ai.NewOpenAIClient(cfg.AI, cfg.Prompts, logger)

	// 初始化规则引擎
	ruleEngine := engine.NewRuleEngine(logger)

	// 初始化仓库
	repo := repository.New(db)

	// 初始化输出工作器（异步处理 OUTPUTTING 阶段）
	outputWorker := worker.NewOutputWorker(db, aiClient, repo, logger, 5)

	// 初始化编排器
	orch := orchestrator.NewTurnOrchestrator(db, aiClient, ruleEngine, repo, outputWorker, logger,
		cfg.Messages.AskingOtherCustomers, cfg.Messages.OutputtingConfirm, cfg.Messages.OutputtingEnded)

	return &Server{
		config:       cfg,
		db:           db,
		feishuClient: feishuClient,
		aiClient:     aiClient,
		orchestrator: orch,
		outputWorker: outputWorker,
		logger:       logger,
	}
}

// securityHeadersMiddleware 为响应添加安全头，防 Clickjacking 等
func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

// Start 启动服务器
func (s *Server) Start() error {
	// 启动输出工作器（异步处理 OUTPUTTING 阶段）
	s.outputWorker.Start()
	s.logger.Info("Output worker started")

	// 启动飞书客户端（WebSocket 会阻塞，必须在独立 goroutine 中运行）
	ctx := context.Background()
	go func() {
		if err := s.feishuClient.Start(ctx, s); err != nil {
			s.logger.Error("Feishu client stopped with error", "error", err)
		}
	}()

	// 启动HTTP服务器（健康检查、page API、静态文件）
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.healthHandler)

	apiP := s.apiPrefix()
	webP := s.webPrefix()

	// config.js 由下方静态文件 handler 内部优先处理（目录中无此文件，需动态生成）

	// Page API：与 page/index.html 前端配套（含飞书 OAuth）
	mux.HandleFunc(apiP+"/feishu/auth", s.feishuAuthHandler)
	mux.HandleFunc(apiP+"/user/info", s.userInfoHandler)
	mux.HandleFunc(apiP+"/records", s.pageAPIRootHandler)
	mux.HandleFunc(apiP+"/records/", s.pageAPISubHandler)

	// 静态页面（records/pages 目录）
	staticDir := s.config.Server.StaticDir
	if staticDir == "" {
		staticDir = "pages"
	}
	if staticDir != "" {
		absDir, err := filepath.Abs(staticDir)
		if err != nil {
			s.logger.Warn("Invalid static_dir, skipping static file serving", "static_dir", s.config.Server.StaticDir, "error", err)
		} else if info, err := os.Stat(absDir); err == nil && info.IsDir() {
			fs := http.FileServer(http.Dir(absDir))
			if webP == "/" {
				// 根路径：config.js 需在静态文件前处理，避免被 FileServer 接管（FileServer 会 404）
				mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/config.js" {
						s.configJSHandler(w, r)
						return
					}
					fs.ServeHTTP(w, r)
				}))
			} else {
				mux.HandleFunc(webP, func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == webP {
						http.Redirect(w, r, webP+"/", http.StatusFound)
						return
					}
					http.NotFound(w, r)
				})
				// config.js 需在静态文件前处理，避免被 FileServer 接管（目录中无该文件会 404）
				mux.Handle(webP+"/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == webP+"/config.js" {
						s.configJSHandler(w, r)
						return
					}
					http.StripPrefix(webP, fs).ServeHTTP(w, r)
				}))
			}
			s.logger.Info("Serving static pages", "dir", absDir, "web_prefix", webP, "api_prefix", apiP)
		} else {
			s.logger.Warn("Pages dir not found or not a directory, skipping", "static_dir", absDir)
		}
	}

	// 安全头中间件：防 Clickjacking、XSS 等
	handler := securityHeadersMiddleware(mux)

	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", s.config.Server.Host, s.config.Server.Port),
		Handler:      handler,
		ReadTimeout:  s.config.Server.ReadTimeout,
		WriteTimeout: s.config.Server.WriteTimeout,
		IdleTimeout:  s.config.Server.IdleTimeout,
	}

	s.logger.Info("Starting HTTP server", "addr", s.httpServer.Addr)
	return s.httpServer.ListenAndServe()
}

// Shutdown 优雅关闭服务器
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("Shutting down output worker...")
	s.outputWorker.Stop()
	s.logger.Info("Output worker stopped")

	if s.httpServer != nil {
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

// HandleMessage 实现 feishu.MessageHandler 接口
func (s *Server) HandleMessage(ctx context.Context, msg *feishu.Message) error {
	s.logger.Info("Processing message", "user_id", msg.UserID, "chat_id", msg.ChatID, "content", msg.Content)

	mu := s.getUserLock(msg.UserID)
	mu.Lock()
	defer mu.Unlock()

	if err := s.ensureUserExists(ctx, msg.UserID, false); err != nil {
		s.logger.Error("Failed to ensure user exists", "error", err, "user_id", msg.UserID)
		return s.feishuClient.SendMessage(ctx, msg.ChatID, s.config.Messages.SystemError)
	}

	reply, err := s.orchestrator.ProcessTurn(ctx, msg.UserID, msg.Content)
	if err != nil {
		s.logger.Error("Failed to process turn", "error", err, "user_id", msg.UserID)
		reply = s.config.Messages.ProcessError
	}

	if err := s.feishuClient.SendMessage(ctx, msg.ChatID, reply); err != nil {
		s.logger.Error("Failed to send reply", "error", err, "chat_id", msg.ChatID)
		return err
	}

	return nil
}

// getUserLock 获取或创建用户级锁
func (s *Server) getUserLock(userID string) *sync.Mutex {
	// 尝试加载已存在的锁
	if lock, ok := s.userLocks.Load(userID); ok {
		return lock.(*sync.Mutex)
	}

	// 创建新锁
	newLock := &sync.Mutex{}
	actual, loaded := s.userLocks.LoadOrStore(userID, newLock)
	if loaded {
		// 另一个 goroutine 已经创建了锁，使用它
		return actual.(*sync.Mutex)
	}
	return newLock
}

// HandleUserEnter 实现 feishu.MessageHandler 接口
func (s *Server) HandleUserEnter(ctx context.Context, userID, chatID string) error {
	s.logger.Info("User entered chat", "user_id", userID, "chat_id", chatID)

	if err := s.ensureUserExists(ctx, userID, true); err != nil {
		s.logger.Error("Failed to ensure user exists", "error", err, "user_id", userID)
		return s.feishuClient.SendMessage(ctx, chatID, s.config.Messages.SystemError)
	}

	repo := repository.New(s.db)
	user, err := repo.GetUser(ctx, userID)
	if err != nil {
		s.logger.Error("Failed to get user", "error", err, "user_id", userID)
		return s.feishuClient.SendMessage(ctx, chatID, s.config.Messages.SystemError)
	}

	var welcomeMsg string
	if s.isFirstTimeToday(user) {
		// 用户每天第一次进入对话时
		welcomeMsg = s.config.Messages.NewUser
		if err := repo.UpdateUserStartLark(ctx, userID); err != nil {
			s.logger.Error("Failed to update user start lark", "error", err, "user_id", userID)
		}
	} else {
		session, err := repo.GetActiveSession(ctx, userID)
		if err != nil {
			s.logger.Error("Failed to get active session", "error", err, "user_id", userID)
			welcomeMsg = s.config.Messages.WelcomeBack
		} else if session != nil {
			welcomeMsg = s.config.Messages.ContinueSession
		} else {
			welcomeMsg = s.config.Messages.NewDialog
		}
	}

	return s.feishuClient.SendMessage(ctx, chatID, welcomeMsg)
}

// isFirstTimeToday 判断是否为用户当天第一次进入对话
func (s *Server) isFirstTimeToday(user *models.User) bool {
	if user == nil {
		return false
	}
	if user.StartLark == nil {
		return true
	}
	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	return user.StartLark.Before(todayStart)
}

// ensureUserExists 确保用户存在
// checkStatus 是否检查在职状态、电话号码或组织名称发生变化，如果为 false，则不检查
func (s *Server) ensureUserExists(ctx context.Context, userID string, checkStatus bool) error {
	repo := repository.New(s.db)

	// 检查用户是否已存在
	user, err := repo.GetUser(ctx, userID)
	if err != nil {
		return err
	}

	// 如果不检查状态，且用户已存在，则直接返回
	if !checkStatus && user != nil {
		return nil
	}

	// 从飞书获取用户信息
	userInfo, err := s.feishuClient.GetUserInfo(ctx, userID)
	if err != nil {
		return err
	}

	if user != nil {
		// 当在职状态、电话号码或组织名称发生变化时，更新用户信息
		if user.Status != userInfo.Status || user.OrgName != userInfo.OrgName || user.Phone == nil || *user.Phone != userInfo.Mobile {
			user.Status = userInfo.Status
			user.Phone = &userInfo.Mobile
			user.OrgName = userInfo.OrgName
			if err := repo.UpdateUser(ctx, user); err != nil {
				return fmt.Errorf("failed to update user: %w", err)
			}
		}
		return nil // 用户已存在
	}

	// 创建新用户
	newUser := &models.User{
		ID:      userID,
		Name:    userInfo.Name,
		Phone:   &userInfo.Mobile,
		OrgName: userInfo.OrgName,
		Status:  userInfo.Status,
	}

	if userInfo.Mobile != "" {
		newUser.Phone = &userInfo.Mobile
	}

	if err := repo.CreateUser(ctx, newUser); err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}

	s.logger.Info("Created new user", "user_id", userID, "name", userInfo.Name)
	return nil
}

// healthHandler 健康检查处理器
func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	// 检查数据库连接
	if err := s.db.Ping(); err != nil {
		s.logger.Error("Database health check failed", "error", err)
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("Database unavailable"))
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}
