/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package dao

import (
	"testing"

	"file-share-manager/server/internal/model"
)

func TestCollaborationRecentAndSharedQueriesMySQL(t *testing.T) {
	db := openTransactionTestDB(t)
	if err := db.AutoMigrate(
		&model.User{}, &model.UserGroup{}, &model.UserGroupMember{},
		&model.Node{}, &model.NodeACL{}, &model.RecentNodeAccess{},
	); err != nil {
		t.Fatal(err)
	}
	user := model.User{Username: "collaboration-user", PasswordHash: "unused", RealName: "协作用户", Status: 1}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	group := model.UserGroup{WorkspaceID: 7, Name: "研发部", Source: "local"}
	if err := db.Create(&group).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.UserGroupMember{GroupID: group.ID, UserID: user.ID}).Error; err != nil {
		t.Fatal(err)
	}
	node := model.Node{WorkspaceID: 7, Name: "共享目录", NormalizedName: "共享目录", Type: "folder", Status: "active"}
	if err := db.Create(&node).Error; err != nil {
		t.Fatal(err)
	}
	acl := model.NodeACL{WorkspaceID: 7, NodeID: node.ID, SubjectType: "group", SubjectID: group.ID, Effect: "allow", AccessLevel: "read", InheritToChildren: true}
	if err := db.Create(&acl).Error; err != nil {
		t.Fatal(err)
	}

	dao := &CollaborationDAO{db: db}
	grants, err := dao.ListDirectSharedGrants(7, user.ID, []uint{group.ID})
	if err != nil || len(grants) != 1 || grants[0].SourceName != group.Name || grants[0].ID != node.ID {
		t.Fatalf("ListDirectSharedGrants() = %#v, %v", grants, err)
	}
	if err := dao.TouchRecent(7, user.ID, node.ID); err != nil {
		t.Fatal(err)
	}
	if err := dao.TouchRecent(7, user.ID, node.ID); err != nil {
		t.Fatal(err)
	}
	recent, err := dao.ListRecentNodes(7, user.ID)
	if err != nil || len(recent) != 1 || recent[0].ID != node.ID || recent[0].AccessCount != 2 {
		t.Fatalf("ListRecentNodes() = %#v, %v", recent, err)
	}
}
