package feishu

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"records/internal/config"
)

const feishuAPIBase = "https://open.feishu.cn/open-apis"

var (
	appTokenCache struct {
		mu       sync.Mutex
		token    string
		expireAt time.Time
	}
)

// OAuthUserInfo 飞书 OAuth/OIDC 用户信息
type OAuthUserInfo struct {
	Sub       string `json:"sub"`        // 用户唯一标识
	OpenID    string `json:"open_id"`    // 飞书 open_id
	UnionID   string `json:"union_id"`   // 飞书 union_id
	Name      string `json:"name"`       // 姓名
	AvatarURL string `json:"avatar_url"`  // 头像
}

// oauthTokenResp 获取 access_token 的响应
type oauthTokenResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		AccessToken  string `json:"access_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
		RefreshToken string `json:"refresh_token"`
	} `json:"data"`
}

// appTokenResp 获取 app_access_token 的响应
type appTokenResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		AppAccessToken string `json:"app_access_token"`
		Expire         int    `json:"expire"`
	} `json:"data"`
}

// GetAppAccessToken 获取应用 access_token（内部自建应用）
func GetAppAccessToken(ctx context.Context, cfg config.Feishu) (string, error) {
	appTokenCache.mu.Lock()
	defer appTokenCache.mu.Unlock()

	if appTokenCache.token != "" && time.Now().Before(appTokenCache.expireAt) {
		return appTokenCache.token, nil
	}

	body, _ := json.Marshal(map[string]string{
		"app_id":     cfg.AppID,
		"app_secret": cfg.AppSecret,
	})
	req, err := http.NewRequestWithContext(ctx, "POST", feishuAPIBase+"/auth/v3/app_access_token/internal", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request app token: %w", err)
	}
	defer resp.Body.Close()

	var r appTokenResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", fmt.Errorf("decode app token resp: %w", err)
	}
	if r.Code != 0 {
		return "", fmt.Errorf("app token api error: %d %s", r.Code, r.Msg)
	}

	appTokenCache.token = r.Data.AppAccessToken
	appTokenCache.expireAt = time.Now().Add(time.Duration(r.Data.Expire-300) * time.Second)
	return appTokenCache.token, nil
}

// ExchangeCodeForUserToken 用授权码换用户 access_token
func ExchangeCodeForUserToken(ctx context.Context, cfg config.Feishu, code string) (accessToken string, expiresIn int, refreshToken string, err error) {
	appToken, err := GetAppAccessToken(ctx, cfg)
	if err != nil {
		return "", 0, "", err
	}

	body, _ := json.Marshal(map[string]string{
		"grant_type": "authorization_code",
		"code":       code,
	})
	req, err := http.NewRequestWithContext(ctx, "POST", feishuAPIBase+"/authen/v1/oidc/access_token", bytes.NewReader(body))
	if err != nil {
		return "", 0, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+appToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", 0, "", fmt.Errorf("request user token: %w", err)
	}
	defer resp.Body.Close()

	var r oauthTokenResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", 0, "", fmt.Errorf("decode user token resp: %w", err)
	}
	if r.Code != 0 {
		return "", 0, "", fmt.Errorf("user token api error: %d %s", r.Code, r.Msg)
	}

	return r.Data.AccessToken, r.Data.ExpiresIn, r.Data.RefreshToken, nil
}

// GetOAuthUserInfo 通过用户 access_token 获取用户信息
func GetOAuthUserInfo(ctx context.Context, userAccessToken string) (*OAuthUserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", feishuAPIBase+"/authen/v1/user_info", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+userAccessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request user info: %w", err)
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var wrapper struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Sub       string `json:"sub"`
			OpenID    string `json:"open_id"`
			UnionID   string `json:"union_id"`
			Name      string `json:"name"`
			AvatarURL string `json:"avatar_url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &wrapper); err != nil {
		return nil, fmt.Errorf("decode user info resp: %w", err)
	}
	if wrapper.Code != 0 {
		return nil, fmt.Errorf("user info api error: %d %s", wrapper.Code, wrapper.Msg)
	}

	return &OAuthUserInfo{
		Sub:       wrapper.Data.Sub,
		OpenID:    wrapper.Data.OpenID,
		UnionID:   wrapper.Data.UnionID,
		Name:      wrapper.Data.Name,
		AvatarURL: wrapper.Data.AvatarURL,
	}, nil
}
