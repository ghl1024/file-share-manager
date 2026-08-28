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
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"file-share-manager/server/internal/pkg/security"

	ldapclient "github.com/go-ldap/ldap/v3"
)

var ErrNotConfigured = errors.New("ldap is not configured")
var ErrInvalidCredentials = errors.New("invalid ldap credentials")

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
	dial func(ctx context.Context, address string, timeout time.Duration) (*ldapclient.Conn, error)
}

func NewService() *Service {
	return &Service{dial: func(_ context.Context, address string, timeout time.Duration) (*ldapclient.Conn, error) {
		conn, err := ldapclient.DialURL(address, ldapclient.DialWithDialer(&net.Dialer{Timeout: timeout}))
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
	address := ldapAddress(cfg)
	conn, err := s.dial(ctx, address, 10*time.Second)
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
	address := ldapAddress(cfg)
	conn, err := s.dial(ctx, address, 5*time.Second)
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
	address := ldapAddress(cfg)
	conn, err := s.dial(ctx, address, 10*time.Second)
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
	address := ldapAddress(cfg)
	conn, err := s.dial(ctx, address, 10*time.Second)
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

func ldapAddress(cfg Config) string {
	host := strings.TrimSpace(cfg.Host)
	if strings.HasPrefix(host, "ldap://") || strings.HasPrefix(host, "ldaps://") {
		return host
	}
	return fmt.Sprintf("ldap://%s:%d", host, cfg.Port)
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
