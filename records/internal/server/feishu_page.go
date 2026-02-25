package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"records/internal/auth"
	"records/internal/feishu"
	"records/internal/repository"
)

// configJSHandler 返回前端配置（api_prefix、feishu_app_id、feishu_redirect_uri），供 records/pages/index.html 动态加载
func (s *Server) configJSHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	appID := s.config.Feishu.SaleLogs.AppID
	redirectURI := s.config.Feishu.SaleLogs.RedirectURI
	fmt.Fprintf(w, "window.APP_CONFIG={apiPrefix:%q,feishuAppId:%q,feishuRedirectUri:%q};", s.apiPrefix(), appID, redirectURI)
}

// feishuAuthRequest POST /api/feishu/auth 请求体
type feishuAuthRequest struct {
	Code        string `json:"code"`
	RedirectURI string `json:"redirect_uri"` // 必须与授权请求时一致，否则飞书返回 20014
}

func (s *Server) feishuAuthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req feishuAuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writePageJSON(w, http.StatusBadRequest, pageAPIResponse{Success: false, Message: "无效的请求体"})
		return
	}
	if req.Code == "" {
		s.writePageJSON(w, http.StatusBadRequest, pageAPIResponse{Success: false, Message: "缺少授权码"})
		return
	}

	// redirect_uri 必须与飞书开放平台配置完全一致，优先用请求中的，否则用配置
	redirectURI := req.RedirectURI
	if redirectURI == "" {
		redirectURI = s.config.Feishu.SaleLogs.RedirectURI
	}
	if redirectURI == "" {
		s.logger.Error("redirect_uri not configured for sale_logs")
		s.writePageJSON(w, http.StatusInternalServerError, pageAPIResponse{Success: false, Message: "OAuth 配置错误：请检查 config.yml 中 feishu.sale_logs.redirect_uri"})
		return
	}
	accessToken, _, _, userInfo, err := feishu.ExchangeCodeForUserToken(r.Context(), s.config.Feishu.SaleLogs, req.Code, redirectURI)
	if err != nil {
		s.logger.Error("Feishu exchange code failed", "error", err)
		s.writePageJSON(w, http.StatusInternalServerError, pageAPIResponse{Success: false, Message: err.Error()})
		return
	}

	if userInfo == nil {
		userInfo, err = feishu.GetOAuthUserInfo(r.Context(), accessToken)
		if err != nil {
			s.logger.Error("Feishu get user info failed", "error", err)
			s.writePageJSON(w, http.StatusInternalServerError, pageAPIResponse{Success: false, Message: err.Error()})
			return
		}
	}

	// 使用 union_id 作为 user_id，与机器人（sale_agent）解析后的 ID 一致，实现跨应用统一
	userID := userInfo.UnionID
	if userID == "" {
		s.logger.Error("Feishu OAuth returned no union_id")
		s.writePageJSON(w, http.StatusInternalServerError, pageAPIResponse{Success: false, Message: "认证失败：无法获取 union_id，请检查飞书应用权限配置"})
		return
	}
	s.logger.Info("Feishu OAuth user info", "union_id", userID)

	avatarURL := (*string)(nil)
	if userInfo.AvatarURL != "" {
		avatarURL = &userInfo.AvatarURL
	}

	repo := repository.New(s.db)
	if err := repo.EnsureUserFromOAuth(r.Context(), userID, userInfo.Name, avatarURL); err != nil {
		s.logger.Error("Ensure user from OAuth failed", "error", err, "user_id", userID)
		s.writePageJSON(w, http.StatusInternalServerError, pageAPIResponse{Success: false, Message: "认证失败"})
		return
	}

	data := map[string]interface{}{
		"userId": userID,
		"name":   userInfo.Name,
		"avatar": userInfo.AvatarURL,
	}
	if secret := s.config.Server.JWTSecret; secret != "" {
		token, err := auth.Issue(secret, userID)
		if err != nil {
			s.logger.Error("JWT issue failed", "error", err)
			s.writePageJSON(w, http.StatusInternalServerError, pageAPIResponse{Success: false, Message: "认证失败"})
			return
		}
		data["token"] = token
	}

	s.writePageJSON(w, http.StatusOK, pageAPIResponse{Success: true, Data: data})
}

func (s *Server) userInfoHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID := ""
	if secret := s.config.Server.JWTSecret; secret != "" {
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			tokenString := strings.TrimPrefix(authHeader, "Bearer ")
			uid, err := auth.Validate(secret, tokenString)
			if err == nil {
				userID = uid
			}
		}
		if userID == "" && s.config.Server.AllowXUserIDFallback {
			uid := r.Header.Get("x-user-id")
			if !isInvalidUserID(uid) {
				userID = uid
			}
		}
	} else {
		uid := r.Header.Get("x-user-id")
		if !isInvalidUserID(uid) {
			userID = uid
		}
	}
	if userID == "" || isInvalidUserID(userID) {
		s.writePageJSON(w, http.StatusUnauthorized, pageAPIResponse{Success: false, Message: "未登录"})
		return
	}

	repo := repository.New(s.db)
	user, err := repo.GetUser(r.Context(), userID)
	if err != nil || user == nil {
		s.writePageJSON(w, http.StatusUnauthorized, pageAPIResponse{Success: false, Message: "用户不存在"})
		return
	}

	avatarURL := ""
	if user.AvatarURL != nil {
		avatarURL = *user.AvatarURL
	}

	s.writePageJSON(w, http.StatusOK, pageAPIResponse{
		Success: true,
		Data: map[string]interface{}{
			"user_id":    user.ID,
			"name":       user.Name,
			"avatar_url": avatarURL,
		},
	})
}
