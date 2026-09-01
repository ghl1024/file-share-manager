/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package clamav

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"os"
	"strings"
	"testing"
	"time"
)

func TestScannerScanFileClean(t *testing.T) {
	client, server := net.Pipe()
	go func() {
		defer server.Close()
		header := make([]byte, len("zINSTREAM\x00"))
		_, _ = io.ReadFull(server, header)
		for {
			var length [4]byte
			if _, err := io.ReadFull(server, length[:]); err != nil {
				return
			}
			size := binary.BigEndian.Uint32(length[:])
			if size == 0 {
				break
			}
			_, _ = io.CopyN(io.Discard, server, int64(size))
		}
		_, _ = io.WriteString(server, "stream: OK\x00")
	}()

	filePath := t.TempDir() + "/sample.txt"
	if err := os.WriteFile(filePath, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	scanner := &Scanner{dial: func(context.Context, string, string) (net.Conn, error) {
		return client, nil
	}, addr: "clamav:3310", timeout: time.Second}
	result := scanner.ScanFile(context.Background(), filePath)
	if result.Status != StatusClean || !strings.Contains(result.Message, "OK") {
		t.Fatalf("result = %#v", result)
	}
}

func TestScannerScanFileEICARSampleInfected(t *testing.T) {
	// Assemble the standard harmless anti-malware test signature at runtime so
	// source checkouts are not quarantined by endpoint protection.
	eicar := strings.Join([]string{
		"X5O!P%@AP[4\\PZX54(P^)7CC)7}$EICAR-",
		"STANDARD-ANTIVIRUS-TEST-FILE!$H+H*",
	}, "")
	client, server := net.Pipe()
	received := make(chan string, 1)
	go func() {
		defer server.Close()
		header := make([]byte, len("zINSTREAM\x00"))
		_, _ = io.ReadFull(server, header)
		var payload strings.Builder
		for {
			var length [4]byte
			if _, err := io.ReadFull(server, length[:]); err != nil {
				return
			}
			size := binary.BigEndian.Uint32(length[:])
			if size == 0 {
				break
			}
			_, _ = io.CopyN(&payload, server, int64(size))
		}
		received <- payload.String()
		_, _ = io.WriteString(server, "stream: Win.Test.EICAR_HDB-1 FOUND\x00")
	}()

	filePath := t.TempDir() + "/eicar.com.txt"
	if err := os.WriteFile(filePath, []byte(eicar), 0o600); err != nil {
		t.Fatal(err)
	}
	scanner := &Scanner{dial: func(context.Context, string, string) (net.Conn, error) {
		return client, nil
	}, addr: "clamav:3310", timeout: time.Second}
	result := scanner.ScanFile(context.Background(), filePath)
	if result.Status != StatusInfected || !strings.Contains(result.Message, "FOUND") {
		t.Fatalf("result = %#v", result)
	}
	if got := <-received; got != eicar {
		t.Fatalf("scanner payload mismatch: got %d bytes, want %d", len(got), len(eicar))
	}
}

func TestScannerPing(t *testing.T) {
	client, server := net.Pipe()
	go func() {
		defer server.Close()
		command := make([]byte, len("zPING\x00"))
		_, _ = io.ReadFull(server, command)
		_, _ = io.WriteString(server, "PONG\x00")
	}()
	scanner := &Scanner{dial: func(context.Context, string, string) (net.Conn, error) { return client, nil }, addr: "clamav:3310", timeout: time.Second}
	health, err := scanner.Ping(context.Background())
	if err != nil || !health.Reachable {
		t.Fatalf("health = %#v, err = %v", health, err)
	}
}

func TestScannerHealthParsesVersionAndDetectsStaleDatabase(t *testing.T) {
	client, server := net.Pipe()
	go func() {
		defer server.Close()
		command := make([]byte, len("zVERSION\x00"))
		_, _ = io.ReadFull(server, command)
		_, _ = io.WriteString(server, "ClamAV 1.4.3/27300/Mon Jan  1 00:00:00 2024\x00")
	}()
	scanner := &Scanner{dial: func(context.Context, string, string) (net.Conn, error) { return client, nil }, addr: "clamav:3310", timeout: time.Second}
	health, err := scanner.Health(context.Background(), 48)
	if err != nil {
		t.Fatal(err)
	}
	if !health.Reachable || health.Status != "stale" || health.EngineVersion != "1.4.3" || health.VirusDBVersion != "27300" || health.VirusDBUpdatedAt == nil || !health.VirusDBStale {
		t.Fatalf("health = %#v", health)
	}
}

func TestParseVersionResponseRejectsMalformedResponse(t *testing.T) {
	if _, _, _, err := parseVersionResponse("PONG"); err == nil {
		t.Fatal("expected malformed VERSION response to fail")
	}
}
