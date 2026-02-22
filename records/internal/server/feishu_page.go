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

// configJSHandler 返回前端配置（api_prefix），供 index.html 动态加载
func (s *Server) configJSHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	fmt.Fprintf(w, "window.APP_CONFIG={apiPrefix:%q};", s.apiPrefix())
}

// feishuAuthRequest POST /api/feishu/auth 请求体
type feishuAuthRequest struct {
	Code string `json:"code"`
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

	accessToken, _, _, err := feishu.ExchangeCodeForUserToken(r.Context(), s.config.Feishu, req.Code)
	if err != nil {
		s.logger.Error("Feishu exchange code failed", "error", err)
		s.writePageJSON(w, http.StatusInternalServerError, pageAPIResponse{Success: false, Message: err.Error()})
		return
	}

	userInfo, err := feishu.GetOAuthUserInfo(r.Context(), accessToken)
	if err != nil {
		s.logger.Error("Feishu get user info failed", "error", err)
		s.writePageJSON(w, http.StatusInternalServerError, pageAPIResponse{Success: false, Message: err.Error()})
		return
	}

	// 使用 open_id 作为 user_id，与 users 表一致
	userID := userInfo.OpenID
	if userID == "" {
		userID = userInfo.Sub
	}

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
		if userID == "" && s.config.Server.AllowDemoUser {
			userID = "demo_user"
		}
	} else {
		userID = r.Header.Get("x-user-id")
		if userID == "" {
			userID = "demo_user"
		}
	}
	if userID == "" {
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
			"user_id":     user.ID,
			"name":        user.Name,
			"avatar_url": avatarURL,
		},
	})
}
