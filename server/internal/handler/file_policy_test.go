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
	"archive/zip"
	"testing"

	"file-share-manager/server/internal/config"
)

func TestValidateDetectedFile(t *testing.T) {
	original := config.GetConfig()
	defer config.SetTestConfig(original)
	config.SetTestConfig(&config.Config{Upload: config.UploadConfig{
		AllowedExtensions: []string{".pdf", ".md", ".png", ".docx"},
	}})

	tests := []struct {
		name        string
		filename    string
		mime        string
		header      []byte
		wantAllowed bool
	}{
		{name: "pdf", filename: "report.pdf", mime: "application/pdf", header: []byte("%PDF-1.7\n"), wantAllowed: true},
		{name: "disguised pdf", filename: "report.pdf", mime: "text/plain", header: []byte("plain text"), wantAllowed: false},
		{name: "markdown", filename: "README.md", mime: "text/plain", header: []byte("# title\n"), wantAllowed: true},
		{name: "truncated utf8 text", filename: "README.md", mime: "text/plain", header: []byte{'a', 0xe4, 0xb8}, wantAllowed: true},
		{name: "png", filename: "image.png", mime: "image/png", header: []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, wantAllowed: true},
		{name: "disallowed extension", filename: "payload.exe", mime: "application/octet-stream", header: []byte("MZ"), wantAllowed: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateDetectedFile(test.filename, test.mime, test.header)
			if (err == nil) != test.wantAllowed {
				t.Fatalf("validateDetectedFile() error = %v, wantAllowed = %v", err, test.wantAllowed)
			}
		})
	}
}

func TestValidateDetectedFileAllowsConfiguredMismatch(t *testing.T) {
	original := config.GetConfig()
	defer config.SetTestConfig(original)
	config.SetTestConfig(&config.Config{Upload: config.UploadConfig{
		AllowedExtensions: []string{"*"},
		AllowMIMEMismatch: true,
	}})
	if err := validateDetectedFile("payload.exe", "application/octet-stream", []byte("MZ")); err != nil {
		t.Fatalf("validateDetectedFile() error = %v", err)
	}
}

func TestValidateArchivePackage(t *testing.T) {
	validDocx := []*zip.File{
		{FileHeader: zip.FileHeader{Name: "[Content_Types].xml"}},
		{FileHeader: zip.FileHeader{Name: "word/document.xml"}},
	}
	if err := validateArchivePackage(".docx", validDocx); err != nil {
		t.Fatalf("valid DOCX rejected: %v", err)
	}
	invalidDocx := []*zip.File{{FileHeader: zip.FileHeader{Name: "notes.txt"}}}
	if err := validateArchivePackage(".docx", invalidDocx); err == nil {
		t.Fatal("invalid DOCX package was accepted")
	}
	macroDocument := append(validDocx, &zip.File{FileHeader: zip.FileHeader{Name: "WORD/VBAPROJECT.BIN"}})
	if err := validateArchivePackage(".docx", macroDocument); err == nil {
		t.Fatal("macro-bearing document disguised as DOCX was accepted")
	}
	if err := validateArchivePackage(".docm", macroDocument); err != nil {
		t.Fatalf("valid DOCM package rejected: %v", err)
	}
}
