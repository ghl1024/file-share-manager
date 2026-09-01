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
	"testing"
	"time"

	"file-share-manager/server/internal/config"
)

func TestJWT_GenerateAndParse(t *testing.T) {
	// 模拟配置
	cfg := &config.Config{}
	cfg.JWT.Secret = "test_super_secret"
	cfg.JWT.ExpiresHours = 2
	config.SetTestConfig(cfg) // 需要我们在 config 包加个 SetTestConfig

	userID := uint(123)
	username := "admin"
	isSuperAdmin := true
	workspaceID := uint(42)
	authVersion := uint64(7)
	tokenStr, expiresAt, err := GenerateToken(userID, username, isSuperAdmin, &workspaceID, authVersion)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	if tokenStr == "" {
		t.Fatal("GenerateToken returned empty string")
	}

	if expiresAt.Before(time.Now().Add(1 * time.Hour)) {
		t.Fatal("GenerateToken returned incorrect expiresAt")
	}

	// 测试解析
	claims, err := ParseToken(tokenStr)
	if err != nil {
		t.Fatalf("ParseToken failed: %v", err)
	}

	if claims.UserID != userID {
		t.Errorf("expected UserID %v, got %v", userID, claims.UserID)
	}
	if claims.Username != username {
		t.Errorf("expected Username %v, got %v", username, claims.Username)
	}
	if claims.IsSuperAdmin != isSuperAdmin {
		t.Errorf("expected IsSuperAdmin %v, got %v", isSuperAdmin, claims.IsSuperAdmin)
	}
	if claims.WorkspaceID == nil || *claims.WorkspaceID != workspaceID {
		t.Errorf("expected WorkspaceID %v, got %v", workspaceID, claims.WorkspaceID)
	}
	if claims.AuthVersion != authVersion {
		t.Fatalf("expected auth version %d, got %d", authVersion, claims.AuthVersion)
	}

	// 测试无效 token
	_, err = ParseToken(tokenStr + "invalid")
	if err == nil {
		t.Fatal("expected error parsing invalid token, got nil")
	}
}

func TestJWTWorkspaceAccessContext(t *testing.T) {
	cfg := &config.Config{}
	cfg.JWT.Secret = "test_super_secret"
	cfg.JWT.ExpiresHours = 2
	config.SetTestConfig(cfg)
	target := uint(42)
	source := uint(7)

	token, _, err := GenerateTokenWithWorkspaceAccess(1, "admin", true, WorkspaceAccess{
		WorkspaceID: &target, SourceWorkspaceID: &source, CrossWorkspaceAccess: true,
		CrossWorkspaceReason: "security review",
	}, 3)
	if err != nil {
		t.Fatalf("GenerateTokenWithWorkspaceAccess() error = %v", err)
	}
	claims, err := ParseToken(token)
	if err != nil {
		t.Fatalf("ParseToken() error = %v", err)
	}
	if !claims.CrossWorkspaceAccess || claims.SourceWorkspaceID == nil || *claims.SourceWorkspaceID != source ||
		claims.WorkspaceID == nil || *claims.WorkspaceID != target || claims.CrossWorkspaceReason != "security review" {
		t.Fatalf("unexpected workspace access claims: %#v", claims)
	}
}
