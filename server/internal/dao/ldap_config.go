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
	"errors"
	"strings"

	"file-share-manager/server/internal/model"
	"file-share-manager/server/internal/pkg/database"
	ldapservice "file-share-manager/server/internal/service/ldap"

	"gorm.io/gorm"
)

type LDAPConfigDAO struct {
	db *gorm.DB
}

func NewLDAPConfigDAO() *LDAPConfigDAO {
	return &LDAPConfigDAO{db: database.DB}
}

func DefaultLDAPConfig() *model.LDAPConfig {
	return &model.LDAPConfig{
		Port:             389,
		Transport:        "starttls",
		TLSMinVersion:    "1.2",
		UserFilter:       "(&(objectClass=user)(sAMAccountName=*))",
		UsernameAttr:     "sAMAccountName",
		EmailAttr:        "mail",
		RealNameAttr:     "displayName",
		SyncCron:         "0 0 2 * * *",
		GroupSyncEnabled: 0,
		GroupFilter:      "(objectClass=group)",
		GroupNameAttr:    "cn",
		GroupMemberAttr:  "member",
		Status:           0,
	}
}

func (dao *LDAPConfigDAO) Get() (*model.LDAPConfig, error) {
	var cfg model.LDAPConfig
	err := dao.db.Order("id ASC").First(&cfg).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if strings.TrimSpace(cfg.PasswordCiphertext) != "" {
		password, decryptErr := ldapservice.DecryptPassword(cfg.PasswordCiphertext)
		if decryptErr != nil {
			if cfg.Status == 1 || !errors.Is(decryptErr, ldapservice.ErrCredentialKeyMissing) {
				return nil, decryptErr
			}
		} else {
			cfg.Password = password
		}
	}
	applyLDAPDefaults(&cfg)
	return &cfg, nil
}

func (dao *LDAPConfigDAO) Save(cfg *model.LDAPConfig) error {
	return dao.SaveWithAudit(cfg, nil)
}

func (dao *LDAPConfigDAO) SaveWithAudit(cfg *model.LDAPConfig, event *model.OperationLog) error {
	if cfg == nil {
		return errors.New("LDAP 配置不能为空")
	}
	applyLDAPDefaults(cfg)
	return dao.db.Transaction(func(tx *gorm.DB) error {
		var existing model.LDAPConfig
		err := tx.Order("id ASC").First(&existing).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				if cfg.Status == 1 && strings.TrimSpace(cfg.Password) == "" {
					return errors.New("启用 LDAP 时管理员密码不能为空")
				}
				ciphertext, encryptErr := ldapservice.EncryptPassword(cfg.Password)
				if encryptErr != nil {
					return encryptErr
				}
				cfg.PasswordCiphertext = ciphertext
				if err := tx.Create(cfg).Error; err != nil {
					return err
				}
				if event != nil {
					event.TargetType, event.TargetID, event.TargetName = "ldap_config", "1", "LDAP 配置"
				}
				return appendAuditEvent(tx, event, nil, ldapAuditSnapshot(cfg))
			}
			return err
		}
		before := ldapAuditSnapshot(&existing)
		cfg.ID = existing.ID
		cfg.CreatedAt = existing.CreatedAt
		if strings.TrimSpace(cfg.Password) == "" {
			cfg.Password = existing.Password
			if cfg.Status == 0 {
				cfg.PasswordCiphertext = existing.PasswordCiphertext
			} else if strings.TrimSpace(cfg.Password) == "" && strings.TrimSpace(existing.PasswordCiphertext) != "" {
				password, decryptErr := ldapservice.DecryptPassword(existing.PasswordCiphertext)
				if decryptErr != nil {
					return decryptErr
				}
				cfg.Password = password
			}
		}
		if strings.TrimSpace(cfg.Password) != "" {
			ciphertext, encryptErr := ldapservice.EncryptPassword(cfg.Password)
			if encryptErr != nil {
				return encryptErr
			}
			cfg.PasswordCiphertext = ciphertext
		} else {
			if cfg.Status == 1 && strings.TrimSpace(existing.PasswordCiphertext) == "" {
				return errors.New("启用 LDAP 时管理员密码不能为空")
			}
			cfg.PasswordCiphertext = existing.PasswordCiphertext
		}
		if err := tx.Save(cfg).Error; err != nil {
			return err
		}
		if event != nil {
			event.TargetType, event.TargetID, event.TargetName = "ldap_config", "1", "LDAP 配置"
		}
		return appendAuditEvent(tx, event, before, ldapAuditSnapshot(cfg))
	})
}

func ldapAuditSnapshot(cfg *model.LDAPConfig) map[string]any {
	if cfg == nil {
		return nil
	}
	return map[string]any{
		"id": cfg.ID, "host": cfg.Host, "port": cfg.Port, "admin_dn": cfg.AdminDN,
		"transport": cfg.Transport, "tls_ca_configured": strings.TrimSpace(cfg.TLSCA) != "",
		"tls_server_name": cfg.TLSServerName, "tls_min_version": cfg.TLSMinVersion,
		"base_dn": cfg.BaseDN, "user_filter": cfg.UserFilter, "username_attr": cfg.UsernameAttr,
		"email_attr": cfg.EmailAttr, "real_name_attr": cfg.RealNameAttr, "sync_cron": cfg.SyncCron,
		"sync_workspace_id": cfg.SyncWorkspaceID, "group_sync_enabled": cfg.GroupSyncEnabled,
		"group_base_dn": cfg.GroupBaseDN, "group_filter": cfg.GroupFilter,
		"group_name_attr": cfg.GroupNameAttr, "group_member_attr": cfg.GroupMemberAttr,
		"status": cfg.Status, "password_configured": strings.TrimSpace(cfg.Password) != "" || strings.TrimSpace(cfg.PasswordCiphertext) != "",
	}
}

func (dao *LDAPConfigDAO) RuntimeConfig() (ldapservice.Config, error) {
	cfg, err := dao.Get()
	if err != nil || cfg == nil || cfg.Status != 1 {
		return ldapservice.Config{}, err
	}
	return LDAPRuntimeConfig(cfg), nil
}

func LDAPRuntimeConfig(cfg *model.LDAPConfig) ldapservice.Config {
	if cfg == nil {
		return ldapservice.Config{}
	}
	applyLDAPDefaults(cfg)
	return ldapservice.Config{
		Host:            strings.TrimSpace(cfg.Host),
		Port:            cfg.Port,
		AdminDN:         strings.TrimSpace(cfg.AdminDN),
		Password:        cfg.Password,
		Transport:       strings.TrimSpace(cfg.Transport),
		TLSCA:           cfg.TLSCA,
		TLSServerName:   strings.TrimSpace(cfg.TLSServerName),
		TLSMinVersion:   strings.TrimSpace(cfg.TLSMinVersion),
		BaseDN:          strings.TrimSpace(cfg.BaseDN),
		UserFilter:      strings.TrimSpace(cfg.UserFilter),
		UsernameAttr:    strings.TrimSpace(cfg.UsernameAttr),
		EmailAttr:       strings.TrimSpace(cfg.EmailAttr),
		RealNameAttr:    strings.TrimSpace(cfg.RealNameAttr),
		GroupBaseDN:     strings.TrimSpace(cfg.GroupBaseDN),
		GroupFilter:     strings.TrimSpace(cfg.GroupFilter),
		GroupNameAttr:   strings.TrimSpace(cfg.GroupNameAttr),
		GroupMemberAttr: strings.TrimSpace(cfg.GroupMemberAttr),
	}
}

func applyLDAPDefaults(cfg *model.LDAPConfig) {
	cfg.Host = strings.TrimSpace(cfg.Host)
	cfg.AdminDN = strings.TrimSpace(cfg.AdminDN)
	cfg.BaseDN = strings.TrimSpace(cfg.BaseDN)
	cfg.UserFilter = strings.TrimSpace(cfg.UserFilter)
	cfg.UsernameAttr = strings.TrimSpace(cfg.UsernameAttr)
	cfg.EmailAttr = strings.TrimSpace(cfg.EmailAttr)
	cfg.RealNameAttr = strings.TrimSpace(cfg.RealNameAttr)
	cfg.GroupBaseDN = strings.TrimSpace(cfg.GroupBaseDN)
	cfg.GroupFilter = strings.TrimSpace(cfg.GroupFilter)
	cfg.GroupNameAttr = strings.TrimSpace(cfg.GroupNameAttr)
	cfg.GroupMemberAttr = strings.TrimSpace(cfg.GroupMemberAttr)
	cfg.Transport = strings.ToLower(strings.TrimSpace(cfg.Transport))
	if cfg.Transport == "" {
		cfg.Transport = "starttls"
		if strings.HasPrefix(strings.ToLower(cfg.Host), "ldaps://") {
			cfg.Transport = "ldaps"
		}
	}
	cfg.TLSCA = strings.TrimSpace(cfg.TLSCA)
	cfg.TLSServerName = strings.TrimSpace(cfg.TLSServerName)
	if cfg.TLSMinVersion == "" {
		cfg.TLSMinVersion = "1.2"
	}
	if cfg.Port <= 0 {
		cfg.Port = 389
	}
	if cfg.UserFilter == "" {
		cfg.UserFilter = "(&(objectClass=user)(sAMAccountName=*))"
	}
	if cfg.UsernameAttr == "" {
		cfg.UsernameAttr = "sAMAccountName"
	}
	if cfg.EmailAttr == "" {
		cfg.EmailAttr = "mail"
	}
	if cfg.RealNameAttr == "" {
		cfg.RealNameAttr = "displayName"
	}
	if cfg.GroupBaseDN == "" {
		cfg.GroupBaseDN = cfg.BaseDN
	}
	if cfg.GroupFilter == "" {
		cfg.GroupFilter = "(objectClass=group)"
	}
	if cfg.GroupNameAttr == "" {
		cfg.GroupNameAttr = "cn"
	}
	if cfg.GroupMemberAttr == "" {
		cfg.GroupMemberAttr = "member"
	}
	if cfg.GroupSyncEnabled != 1 {
		cfg.GroupSyncEnabled = 0
	}
	cfg.SyncCron = strings.TrimSpace(cfg.SyncCron)
	if cfg.SyncCron == "" {
		cfg.SyncCron = "0 0 2 * * *"
	}
	if cfg.Status != 1 {
		cfg.Status = 0
	}
}
