/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package handler

import (
	"net/http/httptest"
	"testing"
	"time"

	"file-share-manager/server/internal/model"
	"file-share-manager/server/internal/service/authorization"

	"github.com/gin-gonic/gin"
)

func TestShareTokenRoundTrip(t *testing.T) {
	token, hash, err := newShareToken()
	if err != nil {
		t.Fatal(err)
	}
	if !validShareToken(token) {
		t.Fatalf("generated token was rejected: %q", token)
	}
	if len(hash) != 64 || hash == token {
		t.Fatalf("unexpected token hash: %q", hash)
	}
	if validShareToken("short-token") || validShareToken(token+"!") {
		t.Fatal("malformed share token accepted")
	}
}

func TestEffectiveShareStatus(t *testing.T) {
	now := time.Now()
	limit := 2
	tests := []struct {
		name  string
		share model.Share
		want  string
	}{
		{name: "active", share: model.Share{Status: "active", ExpiresAt: now.Add(time.Hour)}, want: "active"},
		{name: "revoked wins", share: model.Share{Status: "revoked", ExpiresAt: now.Add(-time.Hour), MaxDownloads: &limit, DownloadCount: limit}, want: "revoked"},
		{name: "expired timestamp", share: model.Share{Status: "active", ExpiresAt: now.Add(-time.Second)}, want: "expired"},
		{name: "expired status", share: model.Share{Status: "expired", ExpiresAt: now.Add(time.Hour)}, want: "expired"},
		{name: "limit exhausted", share: model.Share{Status: "active", ExpiresAt: now.Add(time.Hour), MaxDownloads: &limit, DownloadCount: limit}, want: "exhausted"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := effectiveShareStatus(test.share); got != test.want {
				t.Fatalf("effectiveShareStatus() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestCompleteAncestorPath(t *testing.T) {
	rootID, parentID, nodeID := uint(1), uint(2), uint(3)
	root := model.Node{ID: rootID, Type: "folder"}
	parent := model.Node{ID: parentID, ParentID: &rootID, Type: "folder"}
	node := &model.Node{ID: nodeID, ParentID: &parentID, Type: "file"}

	if !completeAncestorPath(node, []model.Node{root, parent}) {
		t.Fatal("complete ancestor chain was rejected")
	}
	if completeAncestorPath(node, []model.Node{parent}) {
		t.Fatal("ancestor chain missing its root was accepted")
	}
	if completeAncestorPath(node, nil) {
		t.Fatal("ancestor chain missing its direct parent was accepted")
	}
	rootNode := &model.Node{ID: rootID, Type: "folder"}
	if !completeAncestorPath(rootNode, nil) {
		t.Fatal("root node with no ancestors was rejected")
	}
}

func TestRelativeNodePath(t *testing.T) {
	rootID, folderID, fileID := uint(1), uint(2), uint(3)
	root := &model.Node{ID: rootID, Name: "资料"}
	nodes := map[uint]model.Node{
		folderID: {ID: folderID, ParentID: &rootID, Name: "项目"},
		fileID:   {ID: fileID, ParentID: &folderID, Name: "说明.txt"},
	}
	path := relativeNodePath(root, nodes[fileID], nodes)
	if path != "项目/说明.txt" {
		t.Fatalf("unexpected relative path: %q", path)
	}
}

func TestExternalShareVersionAllowed(t *testing.T) {
	tests := []struct {
		name    string
		version *model.FileVersion
		want    bool
	}{
		{name: "clean file", version: &model.FileVersion{ScanStatus: "clean"}, want: true},
		{name: "unscanned file", version: &model.FileVersion{ScanStatus: "unscanned"}},
		{name: "pending file", version: &model.FileVersion{ScanStatus: "pending_scan"}},
		{name: "scan error", version: &model.FileVersion{ScanStatus: "scan_error"}},
		{name: "infected file", version: &model.FileVersion{ScanStatus: "infected"}},
		{name: "encrypted archive", version: &model.FileVersion{ScanStatus: "clean", Encrypted: true}},
		{name: "missing version"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := externalShareVersionAllowed(test.version); got != test.want {
				t.Fatalf("externalShareVersionAllowed() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestParseShareListQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name        string
		query       string
		wantScope   string
		wantStatus  string
		wantMessage bool
	}{
		{name: "defaults to mine", query: "", wantScope: "mine"},
		{name: "workspace filters", query: "?scope=workspace&name=%E5%90%88%E5%90%8C&status=active&creator=zhang&expires_from=2026-08-01T00:00:00Z&expires_to=2026-09-01T00:00:00Z", wantScope: "workspace", wantStatus: "active"},
		{name: "invalid scope", query: "?scope=all", wantMessage: true},
		{name: "invalid status", query: "?status=unknown", wantMessage: true},
		{name: "invalid time", query: "?expires_from=2026-08-01", wantMessage: true},
		{name: "reversed range", query: "?expires_from=2026-09-01T00:00:00Z&expires_to=2026-08-01T00:00:00Z", wantMessage: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			context, _ := gin.CreateTestContext(httptest.NewRecorder())
			context.Request = httptest.NewRequest("GET", "/shares"+test.query, nil)
			scope, filter, message := parseShareListQuery(context)
			if (message != "") != test.wantMessage {
				t.Fatalf("message = %q, want error %v", message, test.wantMessage)
			}
			if test.wantMessage {
				return
			}
			if scope != test.wantScope || filter.Status != test.wantStatus {
				t.Fatalf("scope/status = %q/%q, want %q/%q", scope, filter.Status, test.wantScope, test.wantStatus)
			}
		})
	}
}

func TestCanViewManagedShare(t *testing.T) {
	share := &model.Share{CreatedBy: 7}
	if !canViewManagedShare(authorization.Actor{UserID: 7}, share) {
		t.Fatal("owner cannot view managed share")
	}
	if !canViewManagedShare(authorization.Actor{UserID: 8, WorkspaceRole: "workspace_admin"}, share) {
		t.Fatal("workspace admin cannot view managed share")
	}
	if canViewManagedShare(authorization.Actor{UserID: 8, WorkspaceRole: "member"}, share) {
		t.Fatal("ordinary member can view another owner's managed share")
	}
}
