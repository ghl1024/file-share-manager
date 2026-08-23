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
	"file-share-manager/server/internal/model"
	"file-share-manager/server/internal/pkg/database"
	"file-share-manager/server/internal/pkg/pagination"

	"gorm.io/gorm"
)

type LDAPSyncHistoryDAO struct {
	db *gorm.DB
}

func NewLDAPSyncHistoryDAO() *LDAPSyncHistoryDAO {
	return &LDAPSyncHistoryDAO{db: database.DB}
}

func (dao *LDAPSyncHistoryDAO) Create(history *model.LDAPSyncHistory) error {
	return dao.db.Create(history).Error
}

func (dao *LDAPSyncHistoryDAO) Update(history *model.LDAPSyncHistory) error {
	return dao.db.Save(history).Error
}

func (dao *LDAPSyncHistoryDAO) ListPage(page, pageSize int) (*pagination.PageResult[model.LDAPSyncHistory], error) {
	query := dao.db.Model(&model.LDAPSyncHistory{}).Order("start_time DESC, id DESC")
	return pagination.Paging[model.LDAPSyncHistory](query, page, pageSize)
}
