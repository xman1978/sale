package feishu

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"records/internal/config"
	"records/pkg/logger"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkcontact "github.com/larksuite/oapi-sdk-go/v3/service/contact/v3"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
)

// Client 飞书客户端接口
type Client interface {
	Start(ctx context.Context, messageHandler MessageHandler) error
	SendMessage(ctx context.Context, chatID, content string) error
	GetUserInfo(ctx context.Context, userID, userIDType string) (*UserInfo, error)
	// ResolveToUnionID 当飞书事件仅含 open_id 时，通过 contact API 解析为 union_id（内部使用）
	ResolveToUnionID(ctx context.Context, openID string) (unionID string, err error)
}

// MessageHandler 消息处理器接口
type MessageHandler interface {
	HandleMessage(ctx context.Context, msg *Message) error
	HandleUserEnter(ctx context.Context, userID, chatID string) error
}

// Message 消息结构
type Message struct {
	UserID    string `json:"user_id"`
	ChatID    string `json:"chat_id"`
	Content   string `json:"content"`
	MessageID string `json:"message_id"`
	ChatType  string `json:"chat_type"`
}

// UserInfo 用户信息
type UserInfo struct {
	UserID  string `json:"user_id"`  // 传入的 user_id（open_id 或 union_id）
	UnionID string `json:"union_id"` // 跨应用统一标识，contact API 返回
	Name    string `json:"name"`
	Mobile  string `json:"mobile"`
	OrgName string `json:"org_name"`
	Status  int    `json:"status"`
}

// processedEventsCache 已处理事件的去重缓存，用于忽略飞书超时重推的重复消息
type processedEventsCache struct {
	mu         sync.RWMutex
	cache      map[string]time.Time
	retention  time.Duration
	maxEntries int
}

func newProcessedEventsCache() *processedEventsCache {
	c := &processedEventsCache{
		cache:      make(map[string]time.Time),
		retention:  8 * time.Hour, // 覆盖飞书约 7.1 小时的重推窗口
		maxEntries: 10000,
	}
	go c.cleanupLoop()
	return c
}

func (c *processedEventsCache) isProcessed(id string) bool {
	c.mu.RLock()
	t, ok := c.cache[id]
	c.mu.RUnlock()
	if !ok {
		return false
	}
	if time.Since(t) > c.retention {
		c.mu.Lock()
		delete(c.cache, id)
		c.mu.Unlock()
		return false
	}
	return true
}

func (c *processedEventsCache) markProcessed(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[id] = time.Now()
	if len(c.cache) > c.maxEntries {
		for k, v := range c.cache {
			if time.Since(v) > c.retention {
				delete(c.cache, k)
			}
		}
	}
}

func (c *processedEventsCache) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		c.mu.Lock()
		now := time.Now()
		for k, v := range c.cache {
			if now.Sub(v) > c.retention {
				delete(c.cache, k)
			}
		}
		c.mu.Unlock()
	}
}

// FeishuClient 飞书客户端实现
type FeishuClient struct {
	client          *lark.Client
	config          config.FeishuApp
	logger          logger.Logger
	wsClient        *larkws.Client
	processedEvents *processedEventsCache
}

// NewClient 创建飞书客户端
func NewClient(cfg config.FeishuApp, logger logger.Logger) *FeishuClient {
	client := lark.NewClient(cfg.AppID, cfg.AppSecret)

	return &FeishuClient{
		client:          client,
		config:          cfg,
		logger:          logger,
		processedEvents: newProcessedEventsCache(),
	}
}

// extractUnionID 从事件 UserId 提取 union_id，若无则用 open_id 解析（飞书事件有时仅含 open_id）
func (c *FeishuClient) extractUnionID(ctx context.Context, uid *larkim.UserId) (string, error) {
	if uid != nil && uid.UnionId != nil && *uid.UnionId != "" {
		return *uid.UnionId, nil
	}
	if uid != nil && uid.OpenId != nil && *uid.OpenId != "" {
		return c.ResolveToUnionID(ctx, *uid.OpenId)
	}
	return "", fmt.Errorf("event has no user_id (union_id or open_id)")
}

// Start 启动飞书客户端
func (c *FeishuClient) Start(ctx context.Context, messageHandler MessageHandler) error {
	// 创建事件处理器
	eventHandler := dispatcher.NewEventDispatcher("", "").
		// 用户进入与机器人的会话
		OnP2ChatAccessEventBotP2pChatEnteredV1(func(ctx context.Context, event *larkim.P2ChatAccessEventBotP2pChatEnteredV1) error {
			userID, err := c.extractUnionID(ctx, event.Event.OperatorId)
			if err != nil {
				c.logger.Error("Failed to extract union_id", "error", err)
				return err
			}
			c.logger.Info("User entered chat", "user_id", userID, "chat_id", *event.Event.ChatId)
			return messageHandler.HandleUserEnter(ctx, userID, *event.Event.ChatId)
		}).
		// 接收消息事件
		OnP2MessageReceiveV1(func(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
			// 去重：飞书超时重推会导致同一消息多次推送，使用 message_id 忽略重复
			dedupKey := ""
			if event.Event != nil && event.Event.Message != nil && event.Event.Message.MessageId != nil {
				dedupKey = *event.Event.Message.MessageId
			}
			if dedupKey != "" && c.processedEvents.isProcessed(dedupKey) {
				c.logger.Debug("Duplicate message ignored", "message_id", dedupKey)
				return nil
			}

			userID, err := c.extractUnionID(ctx, event.Event.Sender.SenderId)
			if err != nil {
				c.logger.Error("Failed to extract union_id", "error", err)
				return err
			}
			c.logger.Info("Received message", "user_id", userID, "chat_id", *event.Event.Message.ChatId)

			// 检查消息类型
			if *event.Event.Message.MessageType != "text" {
				c.logger.Warn("Unsupported message type", "type", *event.Event.Message.MessageType)
				err := c.SendMessage(ctx, *event.Event.Message.ChatId, "抱歉，我只能处理文本消息")
				if err == nil && dedupKey != "" {
					c.processedEvents.markProcessed(dedupKey)
				}
				return err
			}

			// 解析消息内容
			var content map[string]string
			if err := json.Unmarshal([]byte(*event.Event.Message.Content), &content); err != nil {
				c.logger.Error("Failed to parse message content", "error", err)
				sendErr := c.SendMessage(ctx, *event.Event.Message.ChatId, "消息解析失败，请重新发送")
				if sendErr == nil && dedupKey != "" {
					c.processedEvents.markProcessed(dedupKey)
				}
				return sendErr
			}

			msg := &Message{
				UserID:    userID,
				ChatID:    *event.Event.Message.ChatId,
				Content:   content["text"],
				MessageID: *event.Event.Message.MessageId,
				ChatType:  *event.Event.Message.ChatType,
			}

			err = messageHandler.HandleMessage(ctx, msg)
			if err == nil && dedupKey != "" {
				c.processedEvents.markProcessed(dedupKey)
			}
			return err
		})

	// 创建WebSocket客户端
	c.wsClient = larkws.NewClient(c.config.AppID, c.config.AppSecret,
		larkws.WithEventHandler(eventHandler),
		larkws.WithLogLevel(larkcore.LogLevelInfo),
	)

	// 启动WebSocket连接
	if err := c.wsClient.Start(ctx); err != nil {
		return fmt.Errorf("failed to start feishu websocket client: %w", err)
	}

	c.logger.Info("Feishu client started successfully")
	return nil
}

// SendMessage 发送文本消息
func (c *FeishuClient) SendMessage(ctx context.Context, chatID, content string) error {
	// 构建消息内容，使用 json.Marshal 处理文本内容的转义，防止下面构造 json 结构体时报错
	escapedContent, _ := json.Marshal(content)

	msgContent := larkim.NewTextMsgBuilder().
		TextLine(string(escapedContent)[1 : len(string(escapedContent))-1]).
		Build()

	// 发送消息
	resp, err := c.client.Im.Message.Create(ctx, larkim.NewCreateMessageReqBuilder().
		ReceiveIdType(larkim.ReceiveIdTypeChatId).
		Body(larkim.NewCreateMessageReqBodyBuilder().
			MsgType(larkim.MsgTypeText).
			ReceiveId(chatID).
			Content(msgContent).
			Build()).
		Build())

	if err != nil {
		c.logger.Error("Failed to send message", "error", err, "chat_id", chatID)
		return fmt.Errorf("failed to send message: %w", err)
	}

	if !resp.Success() {
		c.logger.Error("Send message failed", "code", resp.Code, "msg", resp.Msg, "chat_id", chatID)
		return fmt.Errorf("send message failed: %d %s", resp.Code, resp.Msg)
	}

	c.logger.Debug("Message sent successfully", "chat_id", chatID)
	return nil
}

// 通过组织 ID 获取组织
func (c *FeishuClient) getOrgnameByOrgId(ctx context.Context, orgId string) (string, error) {
	req := larkcontact.NewGetDepartmentReqBuilder().
		DepartmentId(orgId).
		UserIdType(`open_id`).
		DepartmentIdType(`open_department_id`).
		Build()

	// 发起请求
	resp, err := c.client.Contact.V3.Department.Get(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to get org name: %w", err)
	}
	if !resp.Success() {
		return "", fmt.Errorf("failed to get org name: %d", resp.CodeError.Code)
	}

	return *resp.Data.Department.Name, nil
}

// 通过组织 ID 获取父组织名称
func (c *FeishuClient) getParentOrgNameByOrgId(ctx context.Context, orgId string) (string, error) {
	req := larkcontact.NewParentDepartmentReqBuilder().
		UserIdType(`open_id`).
		DepartmentIdType(`open_department_id`).
		DepartmentId(orgId).
		PageSize(10).
		Build()

	resp, err := c.client.Contact.V3.Department.Parent(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to get parent org name: %w", err)
	}
	if !resp.Success() {
		return "", fmt.Errorf("failed to get parent org name: %s", larkcore.Prettify(resp.CodeError))
	}

	orgName := ""
	for _, item := range resp.Data.Items {
		orgName += *item.Name + "."
	}
	orgName = orgName[:len(orgName)-1]

	return orgName, nil
}

// GetUserInfo 获取用户信息；userIDType 为 "open_id" 或 "union_id"，表示 userID 的类型
func (c *FeishuClient) GetUserInfo(ctx context.Context, userID, userIDType string) (*UserInfo, error) {
	if userIDType == "" {
		userIDType = "union_id"
	}
	req := larkcontact.NewGetUserReqBuilder().
		UserId(userID).
		UserIdType(userIDType).
		DepartmentIdType("open_department_id").
		Build()

	resp, err := c.client.Contact.V3.User.Get(ctx, req)
	if err != nil {
		c.logger.Error("Failed to get user info", "error", err, "user_id", userID)
		return nil, fmt.Errorf("failed to get user info: %w", err)
	}

	if !resp.Success() {
		c.logger.Error("Get user info failed", "code", resp.CodeError.Code, "user_id", userID)
		return nil, fmt.Errorf("get user info failed: %d", resp.CodeError.Code)
	}

	user := resp.Data.User
	userInfo := &UserInfo{
		UserID: userID,
		Name:   *user.Name,
		Mobile: *user.Mobile,
	}
	if user.UnionId != nil && *user.UnionId != "" {
		userInfo.UnionID = *user.UnionId
	}

	// 获取用户状态 0在职/1离职
	if *user.Status.IsActivated {
		userInfo.Status = 0
	} else {
		userInfo.Status = 1
	}

	// 获取组织名称
	departmentId := ""
	for _, depId := range user.DepartmentIds {
		departmentId = depId
		if depId != "0" {
			break
		}
	}
	orgName, err := c.getOrgnameByOrgId(ctx, departmentId)
	if err != nil {
		c.logger.Error("Failed to get org name", "error", err, "department_id", departmentId)
	}

	parentOrgName, err := c.getParentOrgNameByOrgId(ctx, departmentId)
	if err != nil {
		c.logger.Error("Failed to get parent org name", "error", err, "department_id", departmentId)
	}

	if parentOrgName != "" {
		userInfo.OrgName = parentOrgName + "." + orgName
	} else {
		userInfo.OrgName = orgName
	}

	return userInfo, nil
}

// ResolveToUnionID 将 open_id（机器人上下文）解析为 union_id，用于跨应用统一用户标识
func (c *FeishuClient) ResolveToUnionID(ctx context.Context, openID string) (string, error) {
	info, err := c.GetUserInfo(ctx, openID, "open_id")
	if err != nil {
		return "", err
	}
	if info.UnionID != "" {
		return info.UnionID, nil
	}
	return openID, nil
}

// GetUserByMobile 通过手机号获取用户ID
func (c *FeishuClient) GetUserByMobile(ctx context.Context, mobile string) (string, error) {
	req := larkcontact.NewBatchGetIdUserReqBuilder().
		UserIdType("open_id").
		Body(larkcontact.NewBatchGetIdUserReqBodyBuilder().
			Mobiles([]string{mobile}).
			IncludeResigned(true).
			Build()).
		Build()

	resp, err := c.client.Contact.V3.User.BatchGetId(ctx, req)
	if err != nil {
		c.logger.Error("Failed to get user by mobile", "error", err, "mobile", mobile)
		return "", fmt.Errorf("failed to get user by mobile: %w", err)
	}

	if !resp.Success() {
		c.logger.Error("Get user by mobile failed", "code", resp.CodeError.Code, "mobile", mobile)
		return "", fmt.Errorf("get user by mobile failed: %d", resp.CodeError.Code)
	}

	if len(resp.Data.UserList) == 0 {
		return "", fmt.Errorf("user not found with mobile: %s", mobile)
	}

	return *resp.Data.UserList[0].UserId, nil
}
