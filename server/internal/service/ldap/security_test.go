/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package ldap

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"file-share-manager/server/internal/config"
)

func TestLDAPPasswordEnvelopeSupportsRotation(t *testing.T) {
	oldKey := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	newKey := base64.StdEncoding.EncodeToString([]byte("abcdef0123456789abcdef0123456789"))
	config.SetTestConfig(&config.Config{LDAPSecurity: config.LDAPSecurityConfig{CredentialEncryptionKey: oldKey}})
	t.Cleanup(func() { config.SetTestConfig(nil) })

	ciphertext, err := EncryptPassword("ldap-admin-password")
	if err != nil {
		t.Fatal(err)
	}
	decoded, decodeErr := base64.StdEncoding.DecodeString(ciphertext)
	if strings.Contains(ciphertext, "ldap-admin-password") || decodeErr != nil || !bytes.HasPrefix(decoded, ldapCredentialEnvelopeMagic) {
		t.Fatalf("LDAP password envelope leaks plaintext or has unexpected encoding: %q", ciphertext)
	}
	config.SetTestConfig(&config.Config{LDAPSecurity: config.LDAPSecurityConfig{
		CredentialEncryptionKey: newKey, PreviousCredentialEncryptionKey: oldKey,
	}})
	plaintext, err := DecryptPassword(ciphertext)
	if err != nil || plaintext != "ldap-admin-password" {
		t.Fatalf("DecryptPassword() = %q, %v", plaintext, err)
	}
	config.SetTestConfig(&config.Config{LDAPSecurity: config.LDAPSecurityConfig{CredentialEncryptionKey: newKey}})
	if _, err := DecryptPassword(ciphertext); err == nil {
		t.Fatal("ciphertext encrypted with retired key was accepted without previous key")
	}
}

func TestLDAPTransportValidationAndAddress(t *testing.T) {
	startTLS := Config{Host: "ldap.example.com", Port: 389, Transport: "starttls", TLSMinVersion: "1.2"}
	if err := ValidateConfig(startTLS, true); err != nil {
		t.Fatalf("starttls validation failed: %v", err)
	}
	if got := ldapAddress(startTLS); got != "ldap://ldap.example.com:389" {
		t.Fatalf("starttls address = %q", got)
	}
	hostPort := startTLS
	hostPort.Host = "ldap.example.com:1389"
	hostPort.Port = 389
	if got := ldapAddress(hostPort); got != "ldap://ldap.example.com:1389" {
		t.Fatalf("host:port address = %q", got)
	}
	ipv6 := startTLS
	ipv6.Host = "[::1]"
	if got := ldapAddress(ipv6); got != "ldap://[::1]:389" {
		t.Fatalf("IPv6 address = %q", got)
	}
	ipv6Port := startTLS
	ipv6Port.Host = "[::1]:1389"
	if got := ldapAddress(ipv6Port); got != "ldap://[::1]:1389" {
		t.Fatalf("IPv6 host:port address = %q", got)
	}
	ldaps := startTLS
	ldaps.Transport = "ldaps"
	ldaps.Port = 636
	if got := ldapAddress(ldaps); got != "ldaps://ldap.example.com:636" {
		t.Fatalf("ldaps address = %q", got)
	}
	uriEndpoint := ldaps
	uriEndpoint.Host = "ldaps://ldap.example.com:1636"
	uriEndpoint.Port = 389
	if got := ldapAddress(uriEndpoint); got != "ldaps://ldap.example.com:1636" {
		t.Fatalf("explicit URI port address = %q", got)
	}
	plain := startTLS
	plain.Transport = "plain"
	if err := ValidateConfig(plain, false); err == nil || !strings.Contains(err.Error(), "禁止明文") {
		t.Fatalf("plain release validation error = %v", err)
	}
	invalidCA := startTLS
	invalidCA.TLSCA = "not a certificate"
	if err := ValidateConfig(invalidCA, true); err == nil || !strings.Contains(err.Error(), "CA") {
		t.Fatalf("invalid CA validation error = %v", err)
	}
	invalidVersion := startTLS
	invalidVersion.TLSMinVersion = "1.1"
	if err := ValidateConfig(invalidVersion, true); err == nil || !strings.Contains(err.Error(), "TLS") {
		t.Fatalf("invalid TLS version validation error = %v", err)
	}
	downgrade := startTLS
	downgrade.Host = "ldaps://ldap.example.com"
	downgrade.Transport = "plain"
	if err := ValidateConfig(downgrade, true); err == nil || !strings.Contains(err.Error(), "降级") {
		t.Fatalf("LDAP downgrade validation error = %v", err)
	}
	config.SetTestConfig(&config.Config{LDAPSecurity: config.LDAPSecurityConfig{CredentialEncryptionKey: base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))}})
	t.Cleanup(func() { config.SetTestConfig(nil) })
	tlsConfig, err := buildTLSConfig(Config{Host: "ldap.example.com", Port: 636, Transport: "ldaps", TLSMinVersion: "1.3", TLSServerName: "directory.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if tlsConfig.MinVersion != tls.VersionTLS13 || tlsConfig.ServerName != "directory.example.com" {
		t.Fatalf("TLS config = %#v", tlsConfig)
	}
}

func TestLDAPSecureTransportsAndHostnameVerification(t *testing.T) {
	serverCertificate, caPEM := testLDAPCertificate(t, "localhost")
	serverTLS := &tls.Config{Certificates: []tls.Certificate{serverCertificate}, MinVersion: tls.VersionTLS12}

	ldapsPort, stopLDAPS := startTestLDAPTLSServer(t, serverTLS)
	ldapsConfig := Config{
		Host: "localhost", Port: ldapsPort, Transport: "ldaps", TLSCA: caPEM,
		TLSServerName: "localhost", TLSMinVersion: "1.2",
	}
	conn, err := NewService().openConnection(context.Background(), ldapsConfig, 2*time.Second)
	if err != nil {
		stopLDAPS()
		t.Fatalf("LDAPS connection failed: %v", err)
	}
	_ = conn.Close()
	stopLDAPS()

	wrongNamePort, stopWrongName := startTestLDAPTLSServer(t, serverTLS)
	wrongName := ldapsConfig
	wrongName.Port = wrongNamePort
	wrongName.TLSServerName = "wrong.example.com"
	if _, err := NewService().openConnection(context.Background(), wrongName, 2*time.Second); err == nil {
		stopWrongName()
		t.Fatal("LDAPS accepted a certificate for the wrong hostname")
	}
	stopWrongName()

	startTLSPort, stopStartTLS := startTestLDAPStartTLSServer(t, serverTLS)
	startTLSConfig := ldapsConfig
	startTLSConfig.Port = startTLSPort
	startTLSConfig.Transport = "starttls"
	conn, err = NewService().openConnection(context.Background(), startTLSConfig, 2*time.Second)
	if err != nil {
		stopStartTLS()
		t.Fatalf("StartTLS connection failed: %v", err)
	}
	_ = conn.Close()
	stopStartTLS()
}

func testLDAPCertificate(t *testing.T, serverName string) (tls.Certificate, string) {
	t.Helper()
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "FileShare LDAP Test CA"},
		NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour), IsCA: true,
		BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	serverKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: serverName}, DNSNames: []string{serverName},
		NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour), ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
	}
	serverDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caTemplate, &serverKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	caBlock := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	serverBlock := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverDER})
	keyBlock := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(serverKey)})
	certificate, err := tls.X509KeyPair(serverBlock, keyBlock)
	if err != nil {
		t.Fatal(err)
	}
	return certificate, string(caBlock)
}

func startTestLDAPTLSServer(t *testing.T, tlsConfig *tls.Config) (int, func()) {
	t.Helper()
	listener, err := tls.Listen("tcp", "127.0.0.1:0", tlsConfig)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, acceptErr := listener.Accept()
		if acceptErr == nil {
			if tlsConn, ok := conn.(*tls.Conn); ok {
				_ = tlsConn.Handshake()
			}
			_ = conn.Close()
		}
	}()
	return listenerPort(t, listener.Addr()), func() {
		_ = listener.Close()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("test LDAPS server did not stop")
		}
	}
}

func startTestLDAPStartTLSServer(t *testing.T, tlsConfig *tls.Config) (int, func()) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close()
		request := make([]byte, 1024)
		if _, readErr := conn.Read(request); readErr != nil {
			return
		}
		response := []byte{0x30, 0x0c, 0x02, 0x01, 0x01, 0x78, 0x07, 0x0a, 0x01, 0x00, 0x04, 0x00, 0x04, 0x00}
		if _, writeErr := conn.Write(response); writeErr != nil {
			return
		}
		tlsConn := tls.Server(conn, tlsConfig)
		_ = tlsConn.Handshake()
		_ = tlsConn.Close()
	}()
	return listenerPort(t, listener.Addr()), func() {
		_ = listener.Close()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("test StartTLS server did not stop")
		}
	}
}

func listenerPort(t *testing.T, address net.Addr) int {
	t.Helper()
	_, rawPort, err := net.SplitHostPort(address.String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(rawPort)
	if err != nil {
		t.Fatal(err)
	}
	return port
}
