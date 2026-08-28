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
	"testing"

	"file-share-manager/server/internal/model"
)

func TestBatchRelativePath(t *testing.T) {
	rootID, folderID, fileID := uint(1), uint(2), uint(3)
	root := &model.Node{ID: rootID, Name: "资料"}
	nodes := map[uint]model.Node{
		folderID: {ID: folderID, ParentID: &rootID, Name: "项目"},
		fileID:   {ID: fileID, ParentID: &folderID, Name: "说明.txt"},
	}
	if got := batchRelativePath(root, nodes[fileID], nodes); got != "项目/说明.txt" {
		t.Fatalf("batchRelativePath() = %q", got)
	}
}

func TestUniqueArchivePath(t *testing.T) {
	used := map[string]struct{}{}
	values := []string{
		uniqueArchivePath("资料/说明.txt", used),
		uniqueArchivePath("资料/说明.txt", used),
		uniqueArchivePath("资料/说明.txt", used),
	}
	want := []string{"资料/说明.txt", "资料/说明 (2).txt", "资料/说明 (3).txt"}
	for index := range want {
		if values[index] != want[index] {
			t.Fatalf("path %d = %q, want %q", index, values[index], want[index])
		}
	}
}
