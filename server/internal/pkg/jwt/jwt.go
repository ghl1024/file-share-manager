/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package jwt

import (
	"errors"
	"net/http"
	"net/url"
	"time"

	"file-share-manager/server/internal/config"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

const SessionCookieName = "fileshare_session"

// Claims JWT声明
type Claims struct {
	UserID               uint   `json:"user_id"`
	Username             string `json:"username"`
	IsSuperAdmin         bool   `json:"is_super_admin"`
	WorkspaceID          *uint  `json:"workspace_id,omitempty"`
	SourceWorkspaceID    *uint  `json:"source_workspace_id,omitempty"`
	CrossWorkspaceAccess bool   `json:"cross_workspace_access,omitempty"`
	CrossWorkspaceReason string `json:"cross_workspace_reason,omitempty"`
	AuthVersion          uint64 `json:"auth_version"`
	jwt.RegisteredClaims
}

type WorkspaceAccess struct {
	WorkspaceID          *uint
	SourceWorkspaceID    *uint
	CrossWorkspaceAccess bool
	CrossWorkspaceReason string
}

// GenerateToken 生成JWT Token
func GenerateToken(userID uint, username string, isSuperAdmin bool, workspaceID *uint, authVersion uint64) (string, time.Time, error) {
	return GenerateTokenWithWorkspaceAccess(userID, username, isSuperAdmin, WorkspaceAccess{WorkspaceID: workspaceID}, authVersion)
}

func GenerateTokenWithWorkspaceAccess(userID uint, username string, isSuperAdmin bool, access WorkspaceAccess, authVersion uint64) (string, time.Time, error) {
	cfg := config.GetConfig()
	expiresAt := time.Now().Add(time.Duration(cfg.JWT.ExpiresHours) * time.Hour)

	claims := Claims{
		UserID:               userID,
		Username:             username,
		IsSuperAdmin:         isSuperAdmin,
		WorkspaceID:          access.WorkspaceID,
		SourceWorkspaceID:    access.SourceWorkspaceID,
		CrossWorkspaceAccess: access.CrossWorkspaceAccess,
		CrossWorkspaceReason: access.CrossWorkspaceReason,
		AuthVersion:          authVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "fileshare",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(cfg.JWT.Secret))
	if err != nil {
		return "", time.Time{}, err
	}

	return tokenString, expiresAt, nil
}

// ParseToken 解析JWT Token
func ParseToken(tokenString string) (*Claims, error) {
	cfg := config.GetConfig()

	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(cfg.JWT.Secret), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithIssuer("fileshare"))

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, errors.New("invalid token")
}

func Now() time.Time {
	return time.Now()
}

func SetSessionCookie(c *gin.Context, token string, maxAge int) {
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(SessionCookieName, token, maxAge, "/api", "", secureCookie(), true)
}

func ClearSessionCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(SessionCookieName, "", -1, "/api", "", secureCookie(), true)
}

func secureCookie() bool {
	cfg := config.GetConfig()
	if cfg == nil {
		return false
	}
	if cfg.Server.Mode == "release" {
		return true
	}
	parsed, err := url.Parse(cfg.Server.WebURL)
	return err == nil && parsed.Scheme == "https"
}
