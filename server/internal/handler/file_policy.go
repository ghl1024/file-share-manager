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
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"file-share-manager/server/internal/config"
)

var zipExtensions = map[string]struct{}{
	".zip": {}, ".docx": {}, ".docm": {}, ".xlsx": {}, ".xlsm": {},
	".pptx": {}, ".pptm": {}, ".odt": {}, ".ods": {}, ".odp": {},
}

func validateUploadExtension(displayName string) error {
	extension := strings.ToLower(filepath.Ext(strings.TrimSpace(displayName)))
	cfg := config.GetConfig()
	if cfg == nil || len(cfg.Upload.AllowedExtensions) == 0 {
		return nil
	}
	for _, allowed := range cfg.Upload.AllowedExtensions {
		if allowed == "*" || extension == allowed {
			return nil
		}
	}
	if extension == "" {
		return errors.New("不允许上传无扩展名文件")
	}
	return fmt.Errorf("不允许上传该文件类型: %s", extension)
}

func validateDetectedFile(displayName, detectedMIME string, header []byte) error {
	if err := validateUploadExtension(displayName); err != nil {
		return err
	}
	cfg := config.GetConfig()
	if cfg != nil && cfg.Upload.AllowMIMEMismatch {
		return nil
	}

	extension := strings.ToLower(filepath.Ext(strings.TrimSpace(displayName)))
	valid := true
	switch extension {
	case ".txt", ".md", ".csv", ".json", ".xml", ".yaml", ".yml", ".log", ".conf", ".ini", ".svg":
		valid = looksLikeText(header)
	case ".rtf":
		valid = bytes.HasPrefix(header, []byte("{\\rtf"))
	case ".pdf":
		valid = bytes.HasPrefix(header, []byte("%PDF-"))
	case ".png":
		valid = bytes.HasPrefix(header, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})
	case ".jpg", ".jpeg":
		valid = bytes.HasPrefix(header, []byte{0xff, 0xd8, 0xff})
	case ".gif":
		valid = bytes.HasPrefix(header, []byte("GIF87a")) || bytes.HasPrefix(header, []byte("GIF89a"))
	case ".webp":
		valid = hasRIFFType(header, "WEBP")
	case ".bmp":
		valid = bytes.HasPrefix(header, []byte("BM"))
	case ".doc", ".xls", ".ppt":
		valid = bytes.HasPrefix(header, []byte{0xd0, 0xcf, 0x11, 0xe0, 0xa1, 0xb1, 0x1a, 0xe1})
	case ".zip", ".docx", ".docm", ".xlsx", ".xlsm", ".pptx", ".pptm", ".odt", ".ods", ".odp":
		valid = isZIPHeader(header)
	case ".gz", ".tgz":
		valid = bytes.HasPrefix(header, []byte{0x1f, 0x8b})
	case ".7z":
		valid = bytes.HasPrefix(header, []byte{0x37, 0x7a, 0xbc, 0xaf, 0x27, 0x1c})
	case ".rar":
		valid = bytes.HasPrefix(header, []byte("Rar!\x1a\x07"))
	case ".tar":
		valid = len(header) >= 262 && string(header[257:262]) == "ustar"
	case ".mp3":
		valid = bytes.HasPrefix(header, []byte("ID3")) || (len(header) >= 2 && header[0] == 0xff && header[1]&0xe0 == 0xe0)
	case ".wav":
		valid = hasRIFFType(header, "WAVE")
	case ".mp4", ".mov", ".m4a":
		valid = len(header) >= 12 && string(header[4:8]) == "ftyp"
	case ".avi":
		valid = hasRIFFType(header, "AVI ")
	case ".mkv", ".webm":
		valid = bytes.HasPrefix(header, []byte{0x1a, 0x45, 0xdf, 0xa3})
	}
	if !valid {
		return fmt.Errorf("文件扩展名与实际内容不匹配: %s (%s)", extension, detectedMIME)
	}
	return nil
}

func validateArchivePackage(extension string, files []*zip.File) error {
	extension = strings.ToLower(extension)
	if _, requiresZIP := zipExtensions[extension]; !requiresZIP || extension == ".zip" {
		return nil
	}
	names := make(map[string]struct{}, len(files))
	for _, file := range files {
		names[strings.ToLower(strings.ReplaceAll(file.Name, "\\", "/"))] = struct{}{}
	}
	hasName := func(name string) bool {
		_, ok := names[name]
		return ok
	}
	hasPrefix := func(prefix string) bool {
		for name := range names {
			if strings.HasPrefix(name, prefix) {
				return true
			}
		}
		return false
	}

	valid := false
	switch extension {
	case ".docx", ".docm":
		valid = hasName("[content_types].xml") && hasPrefix("word/")
	case ".xlsx", ".xlsm":
		valid = hasName("[content_types].xml") && hasPrefix("xl/")
	case ".pptx", ".pptm":
		valid = hasName("[content_types].xml") && hasPrefix("ppt/")
	case ".odt", ".ods", ".odp":
		valid = hasName("mimetype") && hasName("content.xml")
	}
	if !valid {
		return fmt.Errorf("文件内容不是有效的 %s 文档包", extension)
	}
	macroPath := map[string]string{
		".docx": "word/vbaproject.bin",
		".xlsx": "xl/vbaproject.bin",
		".pptx": "ppt/vbaproject.bin",
	}[extension]
	if macroPath != "" && hasName(macroPath) {
		return fmt.Errorf("检测到 VBA 宏，文件扩展名必须使用对应的宏文档格式，不能使用 %s", extension)
	}
	return nil
}

func looksLikeText(header []byte) bool {
	if len(header) == 0 {
		return true
	}
	if bytes.HasPrefix(header, []byte{0xff, 0xfe}) || bytes.HasPrefix(header, []byte{0xfe, 0xff}) {
		return true
	}
	if bytes.ContainsRune(header, '\x00') {
		return false
	}
	for trailing := 0; trailing <= 3 && trailing <= len(header); trailing++ {
		if utf8.Valid(header[:len(header)-trailing]) {
			return true
		}
	}
	return false
}

func hasRIFFType(header []byte, kind string) bool {
	return len(header) >= 12 && string(header[:4]) == "RIFF" && string(header[8:12]) == kind
}

func isZIPHeader(header []byte) bool {
	return bytes.HasPrefix(header, []byte("PK\x03\x04")) ||
		bytes.HasPrefix(header, []byte("PK\x05\x06")) ||
		bytes.HasPrefix(header, []byte("PK\x07\x08"))
}
