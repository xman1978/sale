package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims JWT 声明，包含 user_id
type Claims struct {
	UserID string `json:"uid"`
	jwt.RegisteredClaims
}

// Issue 签发 JWT，有效期 24 小时
func Issue(secret string, userID string) (string, error) {
	if secret == "" {
		return "", errors.New("jwt secret is empty")
	}
	now := time.Now()
	claims := &Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// Validate 验证 JWT 并返回 user_id
func Validate(secret, tokenString string) (userID string, err error) {
	if secret == "" || tokenString == "" {
		return "", errors.New("invalid token")
	}
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(*jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	})
	if err != nil {
		return "", err
	}
	if !token.Valid {
		return "", errors.New("token invalid")
	}
	return claims.UserID, nil
}
