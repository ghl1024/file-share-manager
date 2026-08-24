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
	"reflect"
	"strings"
	"testing"

	"file-share-manager/server/internal/dao"
)

func TestNormalizeCommentContent(t *testing.T) {
	content, err := normalizeCommentContent("  第一行\r\n第二行  ")
	if err != nil || content != "第一行\n第二行" {
		t.Fatalf("normalizeCommentContent() = %q, %v", content, err)
	}
	if _, err := normalizeCommentContent(" \t\n "); err == nil {
		t.Fatal("blank comment accepted")
	}
	if _, err := normalizeCommentContent("valid\x00invalid"); err == nil {
		t.Fatal("control character accepted")
	}
	if _, err := normalizeCommentContent(strings.Repeat("中", maxCommentLength+1)); err == nil {
		t.Fatal("oversized comment accepted")
	}
}

func TestExtractCommentMentions(t *testing.T) {
	got := extractCommentMentions("联系 admin@example.com，不要识别邮箱；请看 @alice 和（@张三），再次 @Alice。")
	want := []string{"alice", "张三"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("extractCommentMentions() = %#v, want %#v", got, want)
	}
}

func TestCommentHelpers(t *testing.T) {
	if got := differenceIDs([]uint{2, 3, 4}, []uint{1, 2, 4}); !reflect.DeepEqual(got, []uint{3}) {
		t.Fatalf("differenceIDs() = %#v", got)
	}
	if got := commentExcerpt("第一行\n  第二行", 20); got != "第一行 第二行" {
		t.Fatalf("commentExcerpt() = %q", got)
	}
	if got := activitySummary(dao.NodeActivityRecord{Action: "file:version_created", VersionNo: 7}); got != "上传了版本 7" {
		t.Fatalf("activitySummary() = %q", got)
	}
}
