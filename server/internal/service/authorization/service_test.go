/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package authorization

import (
	"testing"

	"file-share-manager/server/internal/dao"
)

func TestResolveACLPrecedence(t *testing.T) {
	tests := []struct {
		name     string
		entries  []dao.EffectiveACLEntry
		required string
		want     bool
	}{
		{
			name: "nearest scope wins",
			entries: []dao.EffectiveACLEntry{
				{Depth: 0, SubjectType: "group", Effect: "allow", AccessLevel: AccessRead},
				{Depth: 1, SubjectType: "user", Effect: "allow", AccessLevel: AccessAdmin},
			},
			required: AccessReadWrite,
			want:     false,
		},
		{
			name: "direct user overrides group deny at same scope",
			entries: []dao.EffectiveACLEntry{
				{Depth: 0, SubjectType: "group", Effect: "deny", AccessLevel: AccessAdmin},
				{Depth: 0, SubjectType: "user", Effect: "allow", AccessLevel: AccessReadWrite},
			},
			required: AccessReadWrite,
			want:     true,
		},
		{
			name: "deny wins within subject class",
			entries: []dao.EffectiveACLEntry{
				{Depth: 0, SubjectType: "group", Effect: "allow", AccessLevel: AccessAdmin},
				{Depth: 0, SubjectType: "group", Effect: "deny", AccessLevel: AccessRead},
			},
			required: AccessRead,
			want:     false,
		},
		{
			name: "ancestor grant applies when no nearer entry exists",
			entries: []dao.EffectiveACLEntry{
				{Depth: 2, SubjectType: "group", Effect: "allow", AccessLevel: AccessAdmin},
			},
			required: AccessReadWrite,
			want:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Resolve(tt.entries, tt.required); got != tt.want {
				t.Fatalf("Resolve() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestShareCreationRequiresReadWriteAccess(t *testing.T) {
	tests := []struct {
		level string
		want  bool
	}{
		{level: AccessRead, want: false},
		{level: AccessReadWrite, want: true},
		{level: AccessAdmin, want: true},
	}
	for _, test := range tests {
		entries := []dao.EffectiveACLEntry{{Depth: 0, SubjectType: "user", Effect: "allow", AccessLevel: test.level}}
		if got := Resolve(entries, AccessReadWrite); got != test.want {
			t.Fatalf("share access for %q = %v, want %v", test.level, got, test.want)
		}
	}
}

func TestResolveAccessLevel(t *testing.T) {
	entries := []dao.EffectiveACLEntry{
		{Depth: 0, SubjectType: "user", Effect: "allow", AccessLevel: AccessReadWrite},
		{Depth: 0, SubjectType: "group", Effect: "allow", AccessLevel: AccessAdmin},
	}
	if got := ResolveAccessLevel(entries); got != AccessReadWrite {
		t.Fatalf("ResolveAccessLevel() = %q, want %q", got, AccessReadWrite)
	}
	entries[0].Effect = "deny"
	if got := ResolveAccessLevel(entries); got != "" {
		t.Fatalf("ResolveAccessLevel() with direct deny = %q, want empty", got)
	}
}

func TestResolveEffectiveSourcesUsesSamePrecedence(t *testing.T) {
	entries := []dao.EffectiveACLSource{
		{Depth: 0, SubjectType: "user", SubjectID: 7, Effect: "allow", AccessLevel: AccessRead, SubjectName: "当前用户"},
		{Depth: 0, SubjectType: "group", SubjectID: 9, Effect: "allow", AccessLevel: AccessAdmin, SubjectName: "研发部"},
		{Depth: 1, SubjectType: "group", SubjectID: 10, Effect: "allow", AccessLevel: AccessAdmin, SubjectName: "平台部"},
	}
	level, sources := resolveEffectiveSources(entries)
	if level != AccessRead || len(sources) != 1 || sources[0].SubjectID != 7 {
		t.Fatalf("resolveEffectiveSources() level=%q sources=%#v", level, sources)
	}

	entries = []dao.EffectiveACLSource{
		{Depth: 2, SubjectType: "group", SubjectID: 9, Effect: "allow", AccessLevel: AccessReadWrite},
		{Depth: 2, SubjectType: "group", SubjectID: 10, Effect: "allow", AccessLevel: AccessRead},
	}
	level, sources = resolveEffectiveSources(entries)
	if level != AccessReadWrite || len(sources) != 2 {
		t.Fatalf("inherited group sources level=%q sources=%#v", level, sources)
	}
}
