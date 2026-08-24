/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package router

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"file-share-manager/server/internal/config"
	"file-share-manager/server/internal/migration"
	"file-share-manager/server/internal/model"
	"file-share-manager/server/internal/pkg/database"
	filesharejwt "file-share-manager/server/internal/pkg/jwt"
	"file-share-manager/server/internal/pkg/security"

	"github.com/gin-gonic/gin"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestHTTPAuthorizationBoundariesMySQL(t *testing.T) {
	db := openAuthorizationTestDB(t)
	if err := migration.Run(db); err != nil {
		t.Fatalf("migrate temporary authorization database: %v", err)
	}

	previousConfig := config.GetConfig()
	previousDB := database.DB
	previousAuditDB := database.AuditArchiveDB
	t.Cleanup(func() {
		config.SetTestConfig(previousConfig)
		database.DB = previousDB
		database.AuditArchiveDB = previousAuditDB
	})
	rootPath := t.TempDir()
	config.SetTestConfig(&config.Config{
		Server:        config.ServerConfig{Mode: "debug", WebURL: "http://127.0.0.1:39000"},
		Storage:       config.StorageConfig{RootPath: rootPath, StagingPath: rootPath, Mode: "local"},
		Upload:        config.UploadConfig{AllowedExtensions: []string{".txt"}},
		JWT:           config.JWTConfig{Secret: "authorization-integration-secret-32", ExpiresHours: 1},
		Backup:        config.BackupConfig{Type: "local", LocalPath: rootPath},
		BatchDownload: config.BatchDownloadConfig{MaxFiles: 100, MaxTotalBytes: 1 << 30, RetentionHours: 1, WorkerCount: 1},
	})
	database.DB = db
	database.AuditArchiveDB = nil

	fixture := seedAuthorizationFixture(t, db)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	RegisterRoutes(engine)

	tests := []struct {
		name   string
		method string
		path   string
		token  string
		body   string
		want   int
	}{
		{
			name: "forged super-admin claim cannot read system users", method: http.MethodGet,
			path: "/api/fileshare/v1/management/system/users", token: fixture.forgedSuperToken, want: http.StatusForbidden,
		},
		{
			name: "workspace admin cannot read system configuration", method: http.MethodGet,
			path: "/api/fileshare/v1/management/system/configs", token: fixture.adminToken, want: http.StatusForbidden,
		},
		{
			name: "workspace admin cannot create a super administrator", method: http.MethodPost,
			path: "/api/fileshare/v1/management/system/users", token: fixture.adminToken,
			body: `{"username":"elevated-user","password":"StrongPassword123!","real_name":"Elevated","is_super_admin":true}`, want: http.StatusForbidden,
		},
		{
			name: "workspace admin cannot create a workspace", method: http.MethodPost,
			path: "/api/fileshare/v1/management/workspaces", token: fixture.adminToken,
			body: `{"name":"Forbidden workspace","code":"forbidden-workspace"}`, want: http.StatusForbidden,
		},
		{
			name: "workspace admin cannot update workspace quota through super-admin route", method: http.MethodPut,
			path: fmt.Sprintf("/api/fileshare/v1/management/workspaces/%d", fixture.workspace1.ID), token: fixture.adminToken,
			body: `{"quota_bytes":1024}`, want: http.StatusForbidden,
		},
		{
			name: "workspace admin cannot run full workspace restore", method: http.MethodPost,
			path: "/api/fileshare/v1/management/backups/not-found/restore-workspace", token: fixture.adminToken,
			body: `{"name":"Restored","code":"restored-workspace","confirm":true}`, want: http.StatusForbidden,
		},
		{
			name: "forged selected workspace is revalidated against membership", method: http.MethodGet,
			path: "/api/fileshare/v1/management/dashboard/stats", token: fixture.forgedWorkspaceToken, want: http.StatusForbidden,
		},
		{
			name: "workspace admin cannot list members from another workspace", method: http.MethodGet,
			path: fmt.Sprintf("/api/fileshare/v1/management/workspaces/%d/members", fixture.workspace2.ID), token: fixture.adminToken, want: http.StatusForbidden,
		},
		{
			name: "workspace admin cannot add members to another workspace", method: http.MethodPost,
			path: fmt.Sprintf("/api/fileshare/v1/management/workspaces/%d/members", fixture.workspace2.ID), token: fixture.adminToken,
			body: fmt.Sprintf(`{"user_id":%d,"role":"member"}`, fixture.member.ID), want: http.StatusForbidden,
		},
		{
			name: "cross-workspace node is hidden", method: http.MethodGet,
			path: fmt.Sprintf("/api/fileshare/v1/management/nodes/%d/detail", fixture.node2.ID), token: fixture.adminToken, want: http.StatusNotFound,
		},
		{
			name: "cross-workspace ACL is hidden", method: http.MethodGet,
			path: fmt.Sprintf("/api/fileshare/v1/management/folders/%d/acl", fixture.node2.ID), token: fixture.adminToken, want: http.StatusNotFound,
		},
		{
			name: "cross-workspace role is hidden despite functional permission", method: http.MethodGet,
			path: fmt.Sprintf("/api/fileshare/v1/management/roles/%d", fixture.role2.ID), token: fixture.memberToken, want: http.StatusNotFound,
		},
		{
			name: "cross-workspace share cannot be revoked", method: http.MethodPost,
			path: fmt.Sprintf("/api/fileshare/v1/management/shares/%d/revoke", fixture.share2.ID), token: fixture.adminToken,
			body: `{}`, want: http.StatusNotFound,
		},
		{
			name: "cross-workspace share detail is hidden", method: http.MethodGet,
			path: fmt.Sprintf("/api/fileshare/v1/management/shares/%d", fixture.share2.ID), token: fixture.adminToken, want: http.StatusNotFound,
		},
		{
			name: "functional ACL permission does not replace node ACL", method: http.MethodGet,
			path: fmt.Sprintf("/api/fileshare/v1/management/folders/%d/acl", fixture.node1.ID), token: fixture.memberToken, want: http.StatusForbidden,
		},
		{
			name: "functional file permission does not replace node ACL", method: http.MethodGet,
			path: fmt.Sprintf("/api/fileshare/v1/management/nodes/%d/detail", fixture.node1.ID), token: fixture.memberToken, want: http.StatusForbidden,
		},
		{
			name: "folder traversal name is rejected", method: http.MethodPost,
			path: "/api/fileshare/v1/management/folders", token: fixture.adminToken,
			body: `{"name":"../escape"}`, want: http.StatusBadRequest,
		},
		{
			name: "upload traversal name is rejected", method: http.MethodPost,
			path: "/api/fileshare/v1/management/uploads/init", token: fixture.adminToken,
			body: `{"display_name":"../escape.txt","total_size":1048576,"chunk_size":1048576}`, want: http.StatusBadRequest,
		},
		{
			name: "node cannot move to parent from another workspace", method: http.MethodPost,
			path: fmt.Sprintf("/api/fileshare/v1/management/nodes/%d/move", fixture.node1.ID), token: fixture.adminToken,
			body: fmt.Sprintf(`{"parent_id":%d}`, fixture.node2.ID), want: http.StatusNotFound,
		},
		{
			name: "member cannot bind a role from another workspace", method: http.MethodPut,
			path: fmt.Sprintf("/api/fileshare/v1/management/users/%d/roles", fixture.member.ID), token: fixture.memberToken,
			body: fmt.Sprintf(`{"role_ids":[%d]}`, fixture.role2.ID), want: http.StatusBadRequest,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := performAuthorizationRequest(engine, test.method, test.path, test.token, test.body)
			if response.Code != test.want {
				t.Fatalf("status = %d, want %d; response = %s", response.Code, test.want, response.Body.String())
			}
		})
	}

	var elevatedUsers int64
	if err := db.Model(&model.User{}).Where("username = ?", "elevated-user").Count(&elevatedUsers).Error; err != nil {
		t.Fatal(err)
	}
	if elevatedUsers != 0 {
		t.Fatalf("forbidden super-admin request created %d users", elevatedUsers)
	}
	var share model.Share
	if err := db.First(&share, fixture.share2.ID).Error; err != nil {
		t.Fatal(err)
	}
	if share.Status != "active" {
		t.Fatalf("cross-workspace revoke changed share status to %q", share.Status)
	}
	privateDetail := performAuthorizationRequest(engine, http.MethodGet,
		fmt.Sprintf("/api/fileshare/v1/management/shares/%d", fixture.share1.ID), fixture.memberToken, "")
	if privateDetail.Code != http.StatusNotFound {
		t.Fatalf("other owner's share detail status = %d, want %d; response = %s", privateDetail.Code, http.StatusNotFound, privateDetail.Body.String())
	}
	var crossWorkspaceBinding int64
	if err := db.Model(&model.UserRole{}).
		Where("workspace_id = ? AND user_id = ? AND role_id = ?", fixture.workspace1.ID, fixture.member.ID, fixture.role2.ID).
		Count(&crossWorkspaceBinding).Error; err != nil {
		t.Fatal(err)
	}
	if crossWorkspaceBinding != 0 {
		t.Fatalf("cross-workspace role request created %d bindings", crossWorkspaceBinding)
	}
	assertPermissionAwareDashboard(t, db, engine, fixture)
	assertShareOwnershipIsolation(t, engine, fixture)
	assertProfileSelfService(t, db, engine, fixture)

	if err := db.Model(&model.User{}).Where("id = ?", fixture.admin.ID).
		UpdateColumn("auth_version", gorm.Expr("auth_version + 1")).Error; err != nil {
		t.Fatal(err)
	}
	expiredSession := performAuthorizationRequest(engine, http.MethodGet, "/api/fileshare/v1/auth/session", fixture.adminToken, "")
	if expiredSession.Code != http.StatusUnauthorized {
		t.Fatalf("stale auth-version token status = %d, want %d; response = %s", expiredSession.Code, http.StatusUnauthorized, expiredSession.Body.String())
	}

	waitForAuthorizationAudits(t, db, int64(len(tests)-1))
}

func assertProfileSelfService(t *testing.T, db *gorm.DB, engine http.Handler, fixture authorizationFixture) {
	t.Helper()
	profileResponse := performAuthorizationRequest(engine, http.MethodGet, "/api/fileshare/v1/auth/profile", fixture.memberToken, "")
	if profileResponse.Code != http.StatusOK {
		t.Fatalf("profile status = %d; response = %s", profileResponse.Code, profileResponse.Body.String())
	}
	var profileEnvelope struct {
		Data struct {
			Source     string `json:"source"`
			Workspaces []struct {
				WorkspaceID     uint   `json:"workspace_id"`
				MembershipRole  string `json:"membership_role"`
				FunctionalRoles []struct {
					Code string `json:"code"`
				} `json:"functional_roles"`
			} `json:"workspaces"`
		} `json:"data"`
	}
	if err := json.Unmarshal(profileResponse.Body.Bytes(), &profileEnvelope); err != nil {
		t.Fatal(err)
	}
	if profileEnvelope.Data.Source != "local" || len(profileEnvelope.Data.Workspaces) != 1 ||
		profileEnvelope.Data.Workspaces[0].WorkspaceID != fixture.workspace1.ID ||
		profileEnvelope.Data.Workspaces[0].MembershipRole != "member" ||
		len(profileEnvelope.Data.Workspaces[0].FunctionalRoles) != 1 {
		t.Fatalf("profile workspace summary = %s", profileResponse.Body.String())
	}

	updateResponse := performAuthorizationRequest(engine, http.MethodPut, "/api/fileshare/v1/auth/profile", fixture.memberToken,
		`{"real_name":"Updated Member","email":"member@example.test","phone":"13800000000"}`)
	if updateResponse.Code != http.StatusOK {
		t.Fatalf("profile update status = %d; response = %s", updateResponse.Code, updateResponse.Body.String())
	}
	var updated model.User
	if err := db.First(&updated, fixture.member.ID).Error; err != nil {
		t.Fatal(err)
	}
	if updated.RealName != "Updated Member" || updated.Email != "member@example.test" || updated.Phone != "13800000000" {
		t.Fatalf("updated profile = %#v", updated)
	}

	wrongPasswordResponse := performAuthorizationRequest(engine, http.MethodPut, "/api/fileshare/v1/auth/password", fixture.memberToken,
		`{"current_password":"WrongPassword123!","new_password":"NewBoundaryPassword123!"}`)
	if wrongPasswordResponse.Code != http.StatusBadRequest {
		t.Fatalf("wrong current password status = %d, want %d; response = %s", wrongPasswordResponse.Code, http.StatusBadRequest, wrongPasswordResponse.Body.String())
	}
	var beforeChange model.User
	if err := db.First(&beforeChange, fixture.member.ID).Error; err != nil {
		t.Fatal(err)
	}
	if beforeChange.AuthVersion != fixture.member.AuthVersion {
		t.Fatalf("failed password change incremented auth version to %d", beforeChange.AuthVersion)
	}

	changeResponse := performAuthorizationRequest(engine, http.MethodPut, "/api/fileshare/v1/auth/password", fixture.memberToken,
		`{"current_password":"BoundaryPassword123!","new_password":"NewBoundaryPassword123!"}`)
	if changeResponse.Code != http.StatusOK {
		t.Fatalf("password change status = %d; response = %s", changeResponse.Code, changeResponse.Body.String())
	}
	var sessionCookie *http.Cookie
	for _, cookie := range changeResponse.Result().Cookies() {
		if cookie.Name == filesharejwt.SessionCookieName {
			sessionCookie = cookie
			break
		}
	}
	if sessionCookie == nil || sessionCookie.Value == "" {
		t.Fatal("password change did not issue a replacement session cookie")
	}
	staleResponse := performAuthorizationRequest(engine, http.MethodGet, "/api/fileshare/v1/auth/session", fixture.memberToken, "")
	if staleResponse.Code != http.StatusUnauthorized {
		t.Fatalf("old token status = %d, want %d; response = %s", staleResponse.Code, http.StatusUnauthorized, staleResponse.Body.String())
	}
	currentRequest := httptest.NewRequest(http.MethodGet, "/api/fileshare/v1/auth/session", nil)
	currentRequest.AddCookie(sessionCookie)
	currentResponse := httptest.NewRecorder()
	engine.ServeHTTP(currentResponse, currentRequest)
	if currentResponse.Code != http.StatusOK {
		t.Fatalf("replacement cookie status = %d; response = %s", currentResponse.Code, currentResponse.Body.String())
	}
}

func assertPermissionAwareDashboard(t *testing.T, db *gorm.DB, engine http.Handler, fixture authorizationFixture) {
	t.Helper()
	quota := int64(1024)
	if err := db.Model(&model.WorkspaceMembership{}).
		Where("workspace_id = ? AND user_id = ?", fixture.workspace1.ID, fixture.member.ID).
		Updates(map[string]any{"used_bytes": 128, "reserved_bytes": 32, "quota_bytes": quota}).Error; err != nil {
		t.Fatal(err)
	}
	visibleFolder := model.Node{WorkspaceID: fixture.workspace1.ID, Name: "Member Folder", NormalizedName: "member folder", Type: "folder", InheritMode: "inherit", Status: "active", CreatedBy: fixture.member.ID, UpdatedBy: fixture.member.ID}
	if err := db.Create(&visibleFolder).Error; err != nil {
		t.Fatal(err)
	}
	parentID := visibleFolder.ID
	visibleFile := model.Node{WorkspaceID: fixture.workspace1.ID, ParentID: &parentID, Name: "Member File", NormalizedName: "member file", Type: "file", InheritMode: "inherit", Status: "active", CreatedBy: fixture.member.ID, UpdatedBy: fixture.member.ID}
	if err := db.Create(&visibleFile).Error; err != nil {
		t.Fatal(err)
	}
	closures := []model.NodeClosure{
		{AncestorID: visibleFolder.ID, DescendantID: visibleFolder.ID, Depth: 0},
		{AncestorID: visibleFile.ID, DescendantID: visibleFile.ID, Depth: 0},
		{AncestorID: visibleFolder.ID, DescendantID: visibleFile.ID, Depth: 1},
	}
	if err := db.Create(&closures).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.NodeACL{
		WorkspaceID: fixture.workspace1.ID, NodeID: visibleFolder.ID, SubjectType: "user", SubjectID: fixture.member.ID,
		Effect: "allow", AccessLevel: "read", InheritToChildren: true, CreatedBy: fixture.admin.ID,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&[]model.Favorite{
		{WorkspaceID: fixture.workspace1.ID, UserID: fixture.member.ID, NodeID: visibleFile.ID},
		{WorkspaceID: fixture.workspace1.ID, UserID: fixture.member.ID, NodeID: fixture.node1.ID},
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&[]model.RecentNodeAccess{
		{WorkspaceID: fixture.workspace1.ID, UserID: fixture.member.ID, NodeID: visibleFile.ID, AccessCount: 1, LastAccessedAt: time.Now()},
		{WorkspaceID: fixture.workspace1.ID, UserID: fixture.member.ID, NodeID: fixture.node1.ID, AccessCount: 1, LastAccessedAt: time.Now().Add(-time.Minute)},
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Share{
		WorkspaceID: fixture.workspace1.ID, SourceNodeID: visibleFile.ID, PublicID: uuid.NewString(), TokenHash: strings.Repeat("c", 64),
		Name: "Member share", RootType: "file", RootName: visibleFile.Name, ExpiresAt: time.Now().Add(time.Hour), Status: "active", CreatedBy: fixture.member.ID,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.UploadSession{
		ID: "dashboard-upload", WorkspaceID: fixture.workspace1.ID, DisplayName: "pending.txt", TotalSize: 32, ChunkSize: 32,
		TotalChunks: 1, Status: "uploading", ExpiresAt: time.Now().Add(time.Hour), CreatedBy: fixture.member.ID,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.BatchDownloadJob{
		ID: uuid.NewString(), WorkspaceID: fixture.workspace1.ID, CreatedBy: fixture.member.ID, Name: "pending.zip", Status: "queued", TotalFiles: 1,
	}).Error; err != nil {
		t.Fatal(err)
	}

	response := performAuthorizationRequest(engine, http.MethodGet, "/api/fileshare/v1/management/dashboard/stats?scope=mine", fixture.memberToken, "")
	if response.Code != http.StatusOK {
		t.Fatalf("member dashboard status = %d; response = %s", response.Code, response.Body.String())
	}
	var envelope struct {
		Data struct {
			ViewScope        string           `json:"view_scope"`
			QuotaSource      string           `json:"quota_source"`
			FileCount        int64            `json:"file_count"`
			FolderCount      int64            `json:"folder_count"`
			ActiveShareCount int64            `json:"active_share_count"`
			FavoriteCount    int64            `json:"favorite_count"`
			UsedBytes        int64            `json:"used_bytes"`
			ReservedBytes    int64            `json:"reserved_bytes"`
			QuotaBytes       *int64           `json:"quota_bytes"`
			CanViewWorkspace bool             `json:"can_view_workspace"`
			RecentNodes      []map[string]any `json:"recent_nodes"`
			FavoriteNodes    []map[string]any `json:"favorite_nodes"`
			Tasks            struct {
				UploadInProgress   int64 `json:"upload_in_progress"`
				DownloadInProgress int64 `json:"download_in_progress"`
			} `json:"tasks"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	data := envelope.Data
	if data.ViewScope != "mine" || data.CanViewWorkspace || data.FileCount != 1 || data.FolderCount != 1 || data.ActiveShareCount != 1 {
		t.Fatalf("member dashboard leaked workspace data: %#v", data)
	}
	if data.UsedBytes != 128 || data.ReservedBytes != 32 || data.QuotaBytes == nil || *data.QuotaBytes != quota || data.QuotaSource != "personal" {
		t.Fatalf("member dashboard quota = %#v", data)
	}
	if data.FavoriteCount != 1 || len(data.RecentNodes) != 1 || len(data.FavoriteNodes) != 1 || data.Tasks.UploadInProgress != 1 || data.Tasks.DownloadInProgress != 1 {
		t.Fatalf("member dashboard personal summary = %#v", data)
	}
	for _, item := range append(data.RecentNodes, data.FavoriteNodes...) {
		if item["name"] == fixture.node1.Name {
			t.Fatalf("member dashboard leaked inaccessible node: %#v", item)
		}
	}
	workspaceResponse := performAuthorizationRequest(engine, http.MethodGet, "/api/fileshare/v1/management/dashboard/stats?scope=workspace", fixture.memberToken, "")
	if workspaceResponse.Code != http.StatusForbidden {
		t.Fatalf("member workspace dashboard status = %d, want %d; response = %s", workspaceResponse.Code, http.StatusForbidden, workspaceResponse.Body.String())
	}
}

func assertShareOwnershipIsolation(t *testing.T, engine http.Handler, fixture authorizationFixture) {
	t.Helper()
	memberResponse := performAuthorizationRequest(engine, http.MethodGet,
		"/api/fileshare/v1/management/shares?scope=mine", fixture.memberToken, "")
	if memberResponse.Code != http.StatusOK {
		t.Fatalf("member share list status = %d; response = %s", memberResponse.Code, memberResponse.Body.String())
	}
	var memberEnvelope struct {
		Data struct {
			Total int64 `json:"total"`
			List  []struct {
				Name          string `json:"name"`
				CreatedBy     uint   `json:"created_by"`
				CanRevoke     bool   `json:"can_revoke"`
				CanViewSource bool   `json:"can_view_source"`
			} `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(memberResponse.Body.Bytes(), &memberEnvelope); err != nil {
		t.Fatal(err)
	}
	if memberEnvelope.Data.Total != 1 || len(memberEnvelope.Data.List) != 1 {
		t.Fatalf("member share list leaked other owners: %s", memberResponse.Body.String())
	}
	item := memberEnvelope.Data.List[0]
	if item.Name != "Member share" || item.CreatedBy != fixture.member.ID || !item.CanRevoke || !item.CanViewSource {
		t.Fatalf("member share capabilities = %#v", item)
	}
	workspaceResponse := performAuthorizationRequest(engine, http.MethodGet,
		"/api/fileshare/v1/management/shares?scope=workspace", fixture.memberToken, "")
	if workspaceResponse.Code != http.StatusForbidden {
		t.Fatalf("member workspace share list status = %d, want %d; response = %s", workspaceResponse.Code, http.StatusForbidden, workspaceResponse.Body.String())
	}
	adminResponse := performAuthorizationRequest(engine, http.MethodGet,
		"/api/fileshare/v1/management/shares?scope=workspace&creator=workspace-member&status=active", fixture.adminToken, "")
	if adminResponse.Code != http.StatusOK || strings.Contains(adminResponse.Body.String(), fixture.share1.Name) || !strings.Contains(adminResponse.Body.String(), "Member share") {
		t.Fatalf("workspace share filters did not isolate creator: status=%d response=%s", adminResponse.Code, adminResponse.Body.String())
	}
}

type authorizationFixture struct {
	admin, member          model.User
	workspace1, workspace2 model.Workspace
	node1, node2           model.Node
	role2                  model.Role
	share2                 model.Share
	share1                 model.Share
	adminToken             string
	memberToken            string
	forgedSuperToken       string
	forgedWorkspaceToken   string
}

func seedAuthorizationFixture(t *testing.T, db *gorm.DB) authorizationFixture {
	t.Helper()
	passwordHash, err := security.HashPassword("BoundaryPassword123!")
	if err != nil {
		t.Fatal(err)
	}
	fixture := authorizationFixture{
		admin:  model.User{Username: "workspace-admin", PasswordHash: passwordHash, RealName: "Workspace Admin", Status: 1, Source: "local", AuthVersion: 1},
		member: model.User{Username: "workspace-member", PasswordHash: passwordHash, RealName: "Workspace Member", Status: 1, Source: "local", AuthVersion: 1},
	}
	outsider := model.User{Username: "other-admin", PasswordHash: passwordHash, RealName: "Other Admin", Status: 1, Source: "local", AuthVersion: 1}
	for _, user := range []*model.User{&fixture.admin, &fixture.member, &outsider} {
		if err := db.Create(user).Error; err != nil {
			t.Fatal(err)
		}
	}
	fixture.workspace1 = model.Workspace{UUID: uuid.NewString(), Name: "Workspace One", Code: "workspace-one", Status: 1, CreatedBy: fixture.admin.ID}
	fixture.workspace2 = model.Workspace{UUID: uuid.NewString(), Name: "Workspace Two", Code: "workspace-two", Status: 1, CreatedBy: outsider.ID}
	for _, workspace := range []*model.Workspace{&fixture.workspace1, &fixture.workspace2} {
		if err := db.Create(workspace).Error; err != nil {
			t.Fatal(err)
		}
	}
	memberships := []model.WorkspaceMembership{
		{WorkspaceID: fixture.workspace1.ID, UserID: fixture.admin.ID, Role: "workspace_admin", CreatedBy: fixture.admin.ID},
		{WorkspaceID: fixture.workspace1.ID, UserID: fixture.member.ID, Role: "member", CreatedBy: fixture.admin.ID},
		{WorkspaceID: fixture.workspace2.ID, UserID: outsider.ID, Role: "workspace_admin", CreatedBy: outsider.ID},
	}
	if err := db.Create(&memberships).Error; err != nil {
		t.Fatal(err)
	}

	permissions := []model.Permission{
		{Code: "file:list", Name: "List files"},
		{Code: "file:upload", Name: "Upload files"},
		{Code: "file:share:create", Name: "Manage shares"},
		{Code: "acl:manage", Name: "Manage ACL"},
		{Code: "workspace:role:manage", Name: "Manage roles"},
	}
	if err := db.Create(&permissions).Error; err != nil {
		t.Fatal(err)
	}
	memberRole := model.Role{WorkspaceID: fixture.workspace1.ID, Code: "boundary-member", Name: "Boundary member", Status: 1, CreatedBy: fixture.admin.ID}
	fixture.role2 = model.Role{WorkspaceID: fixture.workspace2.ID, Code: "other-role", Name: "Other role", Status: 1, CreatedBy: outsider.ID}
	if err := db.Create(&memberRole).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&fixture.role2).Error; err != nil {
		t.Fatal(err)
	}
	rolePermissions := make([]model.RolePermission, 0, len(permissions))
	for _, permission := range permissions {
		rolePermissions = append(rolePermissions, model.RolePermission{RoleID: memberRole.ID, PermissionID: permission.ID})
	}
	if err := db.Create(&rolePermissions).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.UserRole{WorkspaceID: fixture.workspace1.ID, UserID: fixture.member.ID, RoleID: memberRole.ID}).Error; err != nil {
		t.Fatal(err)
	}

	fixture.node1 = model.Node{WorkspaceID: fixture.workspace1.ID, Name: "Private One", NormalizedName: "private one", Type: "folder", InheritMode: "inherit", Status: "active", CreatedBy: fixture.admin.ID, UpdatedBy: fixture.admin.ID}
	fixture.node2 = model.Node{WorkspaceID: fixture.workspace2.ID, Name: "Private Two", NormalizedName: "private two", Type: "folder", InheritMode: "inherit", Status: "active", CreatedBy: outsider.ID, UpdatedBy: outsider.ID}
	for _, node := range []*model.Node{&fixture.node1, &fixture.node2} {
		if err := db.Create(node).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&model.NodeClosure{AncestorID: node.ID, DescendantID: node.ID, Depth: 0}).Error; err != nil {
			t.Fatal(err)
		}
	}
	fixture.share2 = model.Share{
		WorkspaceID: fixture.workspace2.ID, SourceNodeID: fixture.node2.ID, PublicID: uuid.NewString(),
		TokenHash: strings.Repeat("a", 64), Name: "Other share", RootType: "folder", RootName: fixture.node2.Name,
		ExpiresAt: time.Now().Add(time.Hour), Status: "active", CreatedBy: outsider.ID,
	}
	if err := db.Create(&fixture.share2).Error; err != nil {
		t.Fatal(err)
	}
	fixture.share1 = model.Share{
		WorkspaceID: fixture.workspace1.ID, SourceNodeID: fixture.node1.ID, PublicID: uuid.NewString(),
		TokenHash: strings.Repeat("b", 64), Name: "Private share", RootType: "folder", RootName: fixture.node1.Name,
		ExpiresAt: time.Now().Add(time.Hour), Status: "active", CreatedBy: fixture.admin.ID,
	}
	if err := db.Create(&fixture.share1).Error; err != nil {
		t.Fatal(err)
	}

	fixture.adminToken = generateAuthorizationToken(t, fixture.admin, false, fixture.workspace1.ID)
	fixture.memberToken = generateAuthorizationToken(t, fixture.member, false, fixture.workspace1.ID)
	fixture.forgedSuperToken = generateAuthorizationToken(t, fixture.admin, true, fixture.workspace1.ID)
	fixture.forgedWorkspaceToken = generateAuthorizationToken(t, fixture.admin, false, fixture.workspace2.ID)
	return fixture
}

func generateAuthorizationToken(t *testing.T, user model.User, isSuperAdmin bool, workspaceID uint) string {
	t.Helper()
	token, _, err := filesharejwt.GenerateToken(user.ID, user.Username, isSuperAdmin, &workspaceID, user.AuthVersion)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func performAuthorizationRequest(engine http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("X-Requested-With", "XMLHttpRequest")
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	return response
}

func waitForAuthorizationAudits(t *testing.T, db *gorm.DB, minimum int64) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var count int64
		if err := db.Model(&model.OperationLog{}).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count >= minimum {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	var count int64
	_ = db.Model(&model.OperationLog{}).Count(&count).Error
	t.Fatalf("audit worker persisted %d events, want at least %d", count, minimum)
}

func openAuthorizationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("FILESHARE_TEST_MYSQL_ADMIN_DSN"))
	if dsn == "" {
		t.Skip("set FILESHARE_TEST_MYSQL_ADMIN_DSN to run HTTP authorization boundary tests")
	}
	adminConfig, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse test MySQL DSN: %v", err)
	}
	adminConfig.ParseTime = true
	adminConfig.DBName = ""
	adminDB, err := gorm.Open(mysql.Open(adminConfig.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("connect test MySQL: %v", err)
	}
	databaseName := "fs_http_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	if err := adminDB.Exec(fmt.Sprintf("CREATE DATABASE `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci", databaseName)).Error; err != nil {
		t.Fatalf("create temporary authorization database: %v", err)
	}
	t.Cleanup(func() {
		_ = adminDB.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", databaseName)).Error
		if sqlDB, sqlErr := adminDB.DB(); sqlErr == nil {
			_ = sqlDB.Close()
		}
	})
	testConfig := *adminConfig
	testConfig.DBName = databaseName
	testDB, err := gorm.Open(mysql.Open(testConfig.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("connect temporary authorization database: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, sqlErr := testDB.DB(); sqlErr == nil {
			_ = sqlDB.Close()
		}
	})
	return testDB
}
