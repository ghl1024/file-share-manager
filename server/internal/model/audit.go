/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package model

import "time"

// OperationLog 审计日志表
type OperationLog struct {
	ID                uint      `gorm:"primaryKey" json:"id"`
	StreamKey         string    `gorm:"type:varchar(64);not null;default:'';index:idx_audit_stream_seq,priority:1" json:"-"`
	StreamSeq         uint64    `gorm:"not null;default:0;index:idx_audit_stream_seq,priority:2" json:"stream_seq"`
	ActorType         string    `gorm:"type:varchar(32);not null;default:'user';index" json:"actor_type"`
	UserID            uint      `gorm:"index;not null" json:"user_id"`
	Username          string    `gorm:"type:varchar(64);not null" json:"username"`
	WorkspaceID       *uint     `gorm:"index;comment:'相关工作空间，全局操作为NULL'" json:"workspace_id"`
	ActorWorkspaceID  *uint     `gorm:"index" json:"actor_workspace_id,omitempty"`
	SourceWorkspaceID *uint     `gorm:"index" json:"source_workspace_id,omitempty"`
	TargetWorkspaceID *uint     `gorm:"index" json:"target_workspace_id,omitempty"`
	Method            string    `gorm:"type:varchar(16);not null" json:"method"`
	Path              string    `gorm:"type:varchar(512);not null" json:"path"`
	Action            string    `gorm:"type:varchar(64);comment:'业务操作类型，如 file:download, acl:grant'" json:"action"`
	Category          string    `gorm:"type:varchar(32);index;default:'operation';comment:'operation, access, security'" json:"category"`
	Severity          string    `gorm:"type:varchar(16);index;default:'info';comment:'info, warning, high'" json:"severity"`
	Result            string    `gorm:"type:varchar(16);index;default:'success';comment:'success, failure, denied'" json:"result"`
	ReasonCode        string    `gorm:"type:varchar(64);index" json:"reason_code"`
	NodeID            *uint     `gorm:"index;comment:'操作涉及的具体文件/目录节点'" json:"node_id"`
	TargetType        string    `gorm:"type:varchar(32);index" json:"target_type,omitempty"`
	TargetID          string    `gorm:"type:varchar(128);index" json:"target_id,omitempty"`
	TargetName        string    `gorm:"type:varchar(255)" json:"target_name_snapshot,omitempty"`
	Status            int       `gorm:"not null" json:"status"`
	IP                string    `gorm:"type:varchar(64)" json:"ip"`
	Latency           int64     `gorm:"comment:'耗时(ms)'" json:"latency"`
	ErrorMessage      string    `gorm:"type:text" json:"error_message"`
	Details           string    `gorm:"type:json;comment:'包含请求参数、前/后状态的JSON序列化数据'" json:"details"`
	RequestID         string    `gorm:"type:varchar(128);index" json:"request_id"`
	TraceID           string    `gorm:"type:varchar(128);index" json:"trace_id"`
	UserAgent         string    `gorm:"type:varchar(512)" json:"user_agent"`
	Origin            string    `gorm:"type:varchar(255)" json:"origin,omitempty"`
	BeforeJSON        *string   `gorm:"type:json" json:"before_json,omitempty"`
	AfterJSON         *string   `gorm:"type:json" json:"after_json,omitempty"`
	MetadataJSON      *string   `gorm:"type:json" json:"metadata_json,omitempty"`
	PrevHash          *string   `gorm:"type:char(64);comment:'上一条审计记录的哈希(不可篡改链)'" json:"prev_hash"`
	CurrentHash       *string   `gorm:"type:char(64);comment:'当前记录的哈希'" json:"current_hash"`
	CreatedAt         time.Time `gorm:"index" json:"created_at"`
}

func (OperationLog) TableName() string {
	return "operation_logs"
}

type AuditStream struct {
	StreamKey   string    `gorm:"type:varchar(64);primaryKey" json:"stream_key"`
	WorkspaceID *uint     `gorm:"uniqueIndex" json:"workspace_id,omitempty"`
	NextSeq     uint64    `gorm:"not null;default:1" json:"next_seq"`
	LastHash    string    `gorm:"type:char(64)" json:"last_hash,omitempty"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (AuditStream) TableName() string { return "audit_streams" }
