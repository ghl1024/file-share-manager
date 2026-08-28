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

const (
	AuditCategoryOperation = "operation"
	AuditCategoryAccess    = "access"
	AuditCategorySecurity  = "security"

	AuditSeverityInfo    = "info"
	AuditSeverityWarning = "warning"
	AuditSeverityHigh    = "high"

	AuditResultSuccess = "success"
	AuditResultFailure = "failure"
	AuditResultDenied  = "denied"

	AuditActorUser          = "user"
	AuditActorSystem        = "system"
	AuditActorExternalShare = "external_share"

	AuditReasonInvalidRequest         = "invalid_request"
	AuditReasonAuthenticationRequired = "authentication_required"
	AuditReasonPermissionDenied       = "permission_denied"
	AuditReasonResourceNotFound       = "resource_not_found"
	AuditReasonResourceGone           = "resource_gone"
	AuditReasonConflict               = "conflict"
	AuditReasonRateLimited            = "rate_limited"
	AuditReasonInternalError          = "internal_error"
	AuditReasonInvalidToken           = "invalid_token"
	AuditReasonShareNotFound          = "share_not_found"
	AuditReasonShareExpired           = "share_expired"
	AuditReasonShareRevoked           = "share_revoked"
	AuditReasonPasswordRequired       = "password_required"
	AuditReasonInvalidCredentials     = "invalid_credentials"
	AuditReasonDownloadLimitReached   = "download_limit_reached"
	AuditReasonUnsafeScanStatus       = "unsafe_scan_status"
	AuditReasonArchiveRestoreRequired = "archive_restore_required"
	AuditReasonStorageUnavailable     = "storage_unavailable"
	AuditReasonObjectNotFound         = "object_not_found"
	AuditReasonUnsupportedMediaType   = "unsupported_media_type"
	AuditReasonPreviewTooLarge        = "preview_too_large"
)

func ValidAuditCategory(value string) bool {
	return value == AuditCategoryOperation || value == AuditCategoryAccess || value == AuditCategorySecurity
}

func ValidAuditSeverity(value string) bool {
	return value == AuditSeverityInfo || value == AuditSeverityWarning || value == AuditSeverityHigh
}

func ValidAuditResult(value string) bool {
	return value == AuditResultSuccess || value == AuditResultFailure || value == AuditResultDenied
}

func ValidAuditActorType(value string) bool {
	return value == AuditActorUser || value == AuditActorSystem || value == AuditActorExternalShare
}

func ValidAuditReasonCode(value string) bool {
	switch value {
	case AuditReasonInvalidRequest, AuditReasonAuthenticationRequired, AuditReasonPermissionDenied,
		AuditReasonResourceNotFound, AuditReasonResourceGone, AuditReasonConflict, AuditReasonRateLimited,
		AuditReasonInternalError, AuditReasonInvalidToken, AuditReasonShareNotFound, AuditReasonShareExpired,
		AuditReasonShareRevoked, AuditReasonPasswordRequired, AuditReasonInvalidCredentials,
		AuditReasonDownloadLimitReached, AuditReasonUnsafeScanStatus, AuditReasonArchiveRestoreRequired,
		AuditReasonStorageUnavailable, AuditReasonObjectNotFound, AuditReasonUnsupportedMediaType,
		AuditReasonPreviewTooLarge:
		return true
	default:
		return false
	}
}
