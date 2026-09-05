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
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"file-share-manager/server/internal/config"
	"file-share-manager/server/internal/pkg/security"

	ldapclient "github.com/go-ldap/ldap/v3"
)

var ErrNotConfigured = errors.New("ldap is not configured")
var ErrInvalidCredentials = errors.New("invalid ldap credentials")
var ErrCredentialKeyMissing = errors.New("ldap credential encryption key is not configured")
var ldapCredentialEnvelopeMagic = []byte("FSMLDAP1")

type Identity struct {
	DN       string
	Username string
	Email    string
	RealName string
}

type Group struct {
	DN           string
	Name         string
	MemberValues []string
}

type Config struct {
	Host            string
	Port            int
	AdminDN         string
	Password        string
	Transport       string
	TLSCA           string
	TLSServerName   string
	TLSMinVersion   string
	BaseDN          string
	UserFilter      string
	UsernameAttr    string
	EmailAttr       string
	RealNameAttr    string
	GroupBaseDN     string
	GroupFilter     string
	GroupNameAttr   string
	GroupMemberAttr string
}

func (c Config) Enabled() bool {
	return strings.TrimSpace(c.Host) != "" && strings.TrimSpace(c.BaseDN) != ""
}

type Service struct {
	dial func(ctx context.Context, address string, timeout time.Duration, tlsConfig *tls.Config) (*ldapclient.Conn, error)
}

func NewService() *Service {
	return &Service{dial: func(_ context.Context, address string, timeout time.Duration, tlsConfig *tls.Config) (*ldapclient.Conn, error) {
		options := []ldapclient.DialOpt{ldapclient.DialWithDialer(&net.Dialer{Timeout: timeout})}
		if tlsConfig != nil {
			options = append(options, ldapclient.DialWithTLSConfig(tlsConfig))
		}
		conn, err := ldapclient.DialURL(address, options...)
		return conn, err
	}}
}

// Authenticate performs a service-account search followed by a user bind.
// Passwords and LDAP bind errors are intentionally collapsed to an auth error.
func (s *Service) Authenticate(ctx context.Context, cfg Config, username, password string) (Identity, error) {
	if !cfg.Enabled() {
		return Identity{}, ErrNotConfigured
	}
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return Identity{}, ErrInvalidCredentials
	}
	conn, err := s.openConnection(ctx, cfg, 10*time.Second)
	if err != nil {
		return Identity{}, ErrInvalidCredentials
	}
	defer conn.Close()
	if cfg.AdminDN != "" {
		if err := conn.Bind(cfg.AdminDN, cfg.Password); err != nil {
			return Identity{}, ErrInvalidCredentials
		}
	}
	filter := buildUserFilter(cfg.UserFilter, cfg.UsernameAttr, username)
	request := ldapclient.NewSearchRequest(cfg.BaseDN, ldapclient.ScopeWholeSubtree, ldapclient.NeverDerefAliases, 1, 10, false, filter, []string{cfg.UsernameAttr, cfg.EmailAttr, cfg.RealNameAttr}, nil)
	result, err := conn.Search(request)
	if err != nil || len(result.Entries) != 1 {
		return Identity{}, ErrInvalidCredentials
	}
	entry := result.Entries[0]
	if err := conn.Bind(entry.DN, password); err != nil {
		return Identity{}, ErrInvalidCredentials
	}
	return Identity{DN: entry.DN, Username: firstNonEmpty(entry.GetAttributeValue(cfg.UsernameAttr), username), Email: entry.GetAttributeValue(cfg.EmailAttr), RealName: firstNonEmpty(entry.GetAttributeValue(cfg.RealNameAttr), username)}, nil
}

func (s *Service) TestConnection(ctx context.Context, cfg Config) error {
	if !cfg.Enabled() {
		return ErrNotConfigured
	}
	conn, err := s.openConnection(ctx, cfg, 5*time.Second)
	if err != nil {
		return ErrInvalidCredentials
	}
	defer conn.Close()
	if cfg.AdminDN == "" {
		return nil
	}
	if err := conn.Bind(cfg.AdminDN, cfg.Password); err != nil {
		return ErrInvalidCredentials
	}
	return nil
}

// ListUsers performs a full LDAP user search with server-side paging.
func (s *Service) ListUsers(ctx context.Context, cfg Config) ([]Identity, error) {
	if !cfg.Enabled() {
		return nil, ErrNotConfigured
	}
	conn, err := s.openConnection(ctx, cfg, 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("connect ldap: %w", err)
	}
	defer conn.Close()
	if cfg.AdminDN != "" {
		if err := conn.Bind(cfg.AdminDN, cfg.Password); err != nil {
			return nil, fmt.Errorf("bind ldap admin: %w", err)
		}
	}

	userFilter := strings.TrimSpace(cfg.UserFilter)
	if userFilter == "" {
		userFilter = "(objectClass=*)"
	}
	usernameAttr := firstNonEmpty(cfg.UsernameAttr, "uid")
	emailAttr := firstNonEmpty(cfg.EmailAttr, "mail")
	realNameAttr := firstNonEmpty(cfg.RealNameAttr, "displayName")
	attributes := uniqueNonEmpty(usernameAttr, emailAttr, realNameAttr)

	var users []Identity
	paging := ldapclient.NewControlPaging(500)
	for {
		request := ldapclient.NewSearchRequest(
			cfg.BaseDN,
			ldapclient.ScopeWholeSubtree,
			ldapclient.NeverDerefAliases,
			0,
			30,
			false,
			userFilter,
			attributes,
			[]ldapclient.Control{paging},
		)
		result, err := conn.Search(request)
		if err != nil {
			return nil, fmt.Errorf("search ldap users: %w", err)
		}
		for _, entry := range result.Entries {
			username := strings.TrimSpace(entry.GetAttributeValue(usernameAttr))
			users = append(users, Identity{
				DN:       entry.DN,
				Username: username,
				Email:    strings.TrimSpace(entry.GetAttributeValue(emailAttr)),
				RealName: firstNonEmpty(entry.GetAttributeValue(realNameAttr), username),
			})
		}
		control := ldapclient.FindControl(result.Controls, ldapclient.ControlTypePaging)
		if control == nil {
			break
		}
		cookie := control.(*ldapclient.ControlPaging).Cookie
		if len(cookie) == 0 {
			break
		}
		paging.SetCookie(cookie)
	}
	return users, nil
}

// ListGroups performs a full LDAP group search with server-side paging.
func (s *Service) ListGroups(ctx context.Context, cfg Config) ([]Group, error) {
	if !cfg.Enabled() {
		return nil, ErrNotConfigured
	}
	conn, err := s.openConnection(ctx, cfg, 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("connect ldap: %w", err)
	}
	defer conn.Close()
	if cfg.AdminDN != "" {
		if err := conn.Bind(cfg.AdminDN, cfg.Password); err != nil {
			return nil, fmt.Errorf("bind ldap admin: %w", err)
		}
	}

	groupBaseDN := firstNonEmpty(cfg.GroupBaseDN, cfg.BaseDN)
	groupFilter := strings.TrimSpace(cfg.GroupFilter)
	if groupFilter == "" {
		groupFilter = "(objectClass=group)"
	}
	groupNameAttr := firstNonEmpty(cfg.GroupNameAttr, "cn")
	groupMemberAttr := firstNonEmpty(cfg.GroupMemberAttr, "member")
	attributes := uniqueNonEmpty(groupNameAttr, groupMemberAttr)

	var groups []Group
	paging := ldapclient.NewControlPaging(500)
	for {
		request := ldapclient.NewSearchRequest(
			groupBaseDN,
			ldapclient.ScopeWholeSubtree,
			ldapclient.NeverDerefAliases,
			0,
			30,
			false,
			groupFilter,
			attributes,
			[]ldapclient.Control{paging},
		)
		result, err := conn.Search(request)
		if err != nil {
			return nil, fmt.Errorf("search ldap groups: %w", err)
		}
		for _, entry := range result.Entries {
			memberValues := make([]string, 0, len(entry.GetAttributeValues(groupMemberAttr)))
			for _, value := range entry.GetAttributeValues(groupMemberAttr) {
				if trimmed := strings.TrimSpace(value); trimmed != "" {
					memberValues = append(memberValues, trimmed)
				}
			}
			groups = append(groups, Group{
				DN:           entry.DN,
				Name:         strings.TrimSpace(entry.GetAttributeValue(groupNameAttr)),
				MemberValues: memberValues,
			})
		}
		control := ldapclient.FindControl(result.Controls, ldapclient.ControlTypePaging)
		if control == nil {
			break
		}
		cookie := control.(*ldapclient.ControlPaging).Cookie
		if len(cookie) == 0 {
			break
		}
		paging.SetCookie(cookie)
	}
	return groups, nil
}

func (s *Service) openConnection(ctx context.Context, cfg Config, timeout time.Duration) (*ldapclient.Conn, error) {
	allowPlaintext := true
	if runtimeConfig := config.GetConfig(); runtimeConfig != nil {
		allowPlaintext = runtimeConfig.Server.Mode != "release"
	}
	if err := ValidateConfig(cfg, allowPlaintext); err != nil {
		return nil, err
	}
	transport := normalizeTransport(cfg.Transport)
	var tlsConfig *tls.Config
	if transport != "plain" {
		var err error
		tlsConfig, err = buildTLSConfig(cfg)
		if err != nil {
			return nil, err
		}
	}
	conn, err := s.dial(ctx, ldapAddress(cfg), timeout, tlsConfig)
	if err != nil {
		return nil, err
	}
	if transport == "starttls" {
		if err := conn.StartTLS(tlsConfig); err != nil {
			_ = conn.Close()
			return nil, err
		}
	}
	return conn, nil
}

// ValidateConfig validates transport and TLS settings before a connection is
// attempted. Plain LDAP is retained for controlled development environments.
func ValidateConfig(cfg Config, allowPlaintext bool) error {
	transport := normalizeTransport(cfg.Transport)
	if transport != "plain" && transport != "starttls" && transport != "ldaps" {
		return fmt.Errorf("LDAP 传输模式必须是 plain、starttls 或 ldaps")
	}
	if transport == "plain" && !allowPlaintext {
		return errors.New("release 模式禁止明文 LDAP")
	}
	hostValue := strings.ToLower(strings.TrimSpace(cfg.Host))
	if strings.HasPrefix(hostValue, "ldaps://") && transport != "ldaps" {
		return errors.New("ldaps 地址不能降级为明文或 StartTLS")
	}
	if _, _, err := ldapEndpoint(cfg); err != nil {
		return err
	}
	if transport != "plain" {
		if _, err := buildTLSConfig(cfg); err != nil {
			return err
		}
	}
	return nil
}

func normalizeTransport(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "starttls"
	}
	return value
}

func ldapAddress(cfg Config) string {
	host, port, err := ldapEndpoint(cfg)
	if err != nil {
		return ""
	}
	transport := normalizeTransport(cfg.Transport)
	scheme := "ldap"
	if transport == "ldaps" {
		scheme = "ldaps"
	}
	return fmt.Sprintf("%s://%s", scheme, net.JoinHostPort(host, fmt.Sprint(port)))
}

func ldapHost(cfg Config) (string, error) {
	host, _, err := ldapEndpoint(cfg)
	return host, err
}

func ldapEndpoint(cfg Config) (string, int, error) {
	raw := strings.TrimSpace(cfg.Host)
	if raw == "" {
		return "", 0, errors.New("LDAP 主机地址不能为空")
	}
	port := cfg.Port
	if strings.Contains(raw, "://") {
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Scheme == "" {
			return "", 0, errors.New("LDAP 主机地址格式无效")
		}
		if parsed.Scheme != "ldap" && parsed.Scheme != "ldaps" {
			return "", 0, errors.New("LDAP 主机地址协议必须是 ldap 或 ldaps")
		}
		if parsed.Host == "" || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.User != nil {
			return "", 0, errors.New("LDAP 主机地址格式无效")
		}
		raw = parsed.Hostname()
		if parsed.Port() != "" {
			parsedPort := parsed.Port()
			for _, value := range parsedPort {
				if value < '0' || value > '9' {
					return "", 0, errors.New("LDAP 主机端口格式无效")
				}
			}
			port, err = strconv.Atoi(parsedPort)
			if err != nil {
				return "", 0, errors.New("LDAP 主机端口格式无效")
			}
		}
	}
	if strings.ContainsAny(raw, "/?#@") {
		return "", 0, errors.New("LDAP 主机地址格式无效")
	}
	if host, rawPort, err := net.SplitHostPort(raw); err == nil {
		raw = host
		port, err = strconv.Atoi(rawPort)
		if err != nil {
			return "", 0, errors.New("LDAP 主机端口格式无效")
		}
	}
	raw = strings.Trim(raw, "[]")
	if raw == "" || net.ParseIP(raw) == nil && strings.Contains(raw, ":") {
		return "", 0, errors.New("LDAP 主机地址格式无效")
	}
	if port < 1 || port > 65535 {
		return "", 0, errors.New("LDAP 端口必须在 1 到 65535 之间")
	}
	return raw, port, nil
}

func buildTLSConfig(cfg Config) (*tls.Config, error) {
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	if strings.TrimSpace(cfg.TLSCA) != "" && !pool.AppendCertsFromPEM([]byte(cfg.TLSCA)) {
		return nil, errors.New("LDAP 自定义 CA 证书格式无效")
	}
	minVersion, err := tlsVersion(cfg.TLSMinVersion)
	if err != nil {
		return nil, err
	}
	host, err := ldapHost(cfg)
	if err != nil {
		return nil, err
	}
	serverName := strings.TrimSpace(cfg.TLSServerName)
	if serverName == "" {
		serverName = host
	}
	return &tls.Config{MinVersion: minVersion, RootCAs: pool, ServerName: serverName}, nil
}

func tlsVersion(value string) (uint16, error) {
	switch strings.TrimSpace(value) {
	case "", "1.2":
		return tls.VersionTLS12, nil
	case "1.3":
		return tls.VersionTLS13, nil
	default:
		return 0, errors.New("LDAP 最低 TLS 版本必须是 1.2 或 1.3")
	}
}

func encryptionKeyCandidates() ([][]byte, error) {
	cfg := config.GetConfig()
	if cfg == nil {
		return nil, ErrCredentialKeyMissing
	}
	values := []string{cfg.LDAPSecurity.CredentialEncryptionKey, cfg.LDAPSecurity.PreviousCredentialEncryptionKey}
	keys := make([][]byte, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(value))
		if err != nil || len(key) != 32 {
			return nil, errors.New("LDAP 凭据加密密钥必须是 Base64 编码的 32 字节")
		}
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		return nil, ErrCredentialKeyMissing
	}
	return keys, nil
}

func EncryptPassword(password string) (string, error) {
	if strings.TrimSpace(password) == "" {
		return "", nil
	}
	keys, err := encryptionKeyCandidates()
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(keys[0])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nil, nonce, []byte(password), ldapCredentialEnvelopeMagic)
	envelope := append(append(append([]byte{}, ldapCredentialEnvelopeMagic...), nonce...), ciphertext...)
	return base64.StdEncoding.EncodeToString(envelope), nil
}

func DecryptPassword(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", nil
	}
	envelope, err := base64.StdEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil || len(envelope) < len(ldapCredentialEnvelopeMagic) {
		return "", errors.New("LDAP 凭据密文格式无效")
	}
	if string(envelope[:len(ldapCredentialEnvelopeMagic)]) != string(ldapCredentialEnvelopeMagic) {
		return "", errors.New("LDAP 凭据密文格式无效")
	}
	keys, err := encryptionKeyCandidates()
	if err != nil {
		return "", err
	}
	for _, key := range keys {
		block, blockErr := aes.NewCipher(key)
		if blockErr != nil {
			continue
		}
		gcm, gcmErr := cipher.NewGCM(block)
		if gcmErr != nil {
			continue
		}
		minimum := len(ldapCredentialEnvelopeMagic) + gcm.NonceSize() + gcm.Overhead()
		if len(envelope) < minimum {
			return "", errors.New("LDAP 凭据密文格式无效")
		}
		nonceStart := len(ldapCredentialEnvelopeMagic)
		nonceEnd := nonceStart + gcm.NonceSize()
		plaintext, openErr := gcm.Open(nil, envelope[nonceStart:nonceEnd], envelope[nonceEnd:], ldapCredentialEnvelopeMagic)
		if openErr == nil {
			return string(plaintext), nil
		}
	}
	return "", errors.New("LDAP 凭据密文校验失败")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func uniqueNonEmpty(values ...string) []string {
	seen := make(map[string]bool, len(values))
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		normalized = append(normalized, value)
	}
	return normalized
}

func buildUserFilter(baseFilter, usernameAttr, username string) string {
	baseFilter = strings.TrimSpace(baseFilter)
	if baseFilter == "" {
		baseFilter = "(objectClass=*)"
	}
	usernameAttr = strings.TrimSpace(usernameAttr)
	if usernameAttr == "" {
		usernameAttr = "uid"
	}
	return "(&" + baseFilter + "(" + usernameAttr + "=" + ldapclient.EscapeFilter(strings.TrimSpace(username)) + "))"
}

func NewLDAPPasswordHash() (string, error) {
	var random [18]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	return security.HashPassword("Ldap!" + hex.EncodeToString(random[:]) + "Aa9")
}
