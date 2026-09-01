/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package pagination

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// PageResult is the shared pagination DTO returned by DAO and query helpers.
// The response package serializes it inside the standard API envelope.
type PageResult[T any] struct {
	Total    int64 `json:"total"`
	List     []T   `json:"list"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
}

// Options controls the defaults and upper bound used when parsing list
// parameters. Keeping these rules in one place prevents handlers from
// silently accepting different page sizes.
type Options struct {
	DefaultPage     int
	DefaultPageSize int
	MaxPageSize     int
}

// Paging 泛型分页查询执行函数
// db: 已经拼装好 WHERE 等过滤条件的 gorm.DB 对象
// 自动执行 Count，并在获取数据时叠加 Order、Offset、Limit
func Paging[T any](db *gorm.DB, page, pageSize int) (*PageResult[T], error) {
	var total int64
	var list []T

	// 计算总数
	if err := db.Count(&total).Error; err != nil {
		return nil, err
	}

	// 限制获取数量，如果没有明确 Order，建议调用方在外层先调用 db.Order("id desc")
	if err := db.Offset((page - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, err
	}

	if list == nil {
		list = []T{}
	}

	return &PageResult[T]{
		Total:    total,
		List:     list,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

// ParseGinContext 从 HTTP 请求上下文中提取分页参数
func ParseGinContext(c *gin.Context) (page, pageSize int, keyword string) {
	return ParseGinContextWithOptions(c, Options{
		DefaultPage:     1,
		DefaultPageSize: 10,
		MaxPageSize:     1000,
	})
}

// ParseGinContextWithOptions extracts and normalizes list parameters using
// resource-specific defaults while keeping validation in this package.
func ParseGinContextWithOptions(c *gin.Context, options Options) (page, pageSize int, keyword string) {
	if options.DefaultPage < 1 {
		options.DefaultPage = 1
	}
	if options.DefaultPageSize < 1 {
		options.DefaultPageSize = 10
	}
	if options.MaxPageSize < 1 {
		options.MaxPageSize = 1000
	}
	if options.DefaultPageSize > options.MaxPageSize {
		options.DefaultPageSize = options.MaxPageSize
	}

	page, _ = strconv.Atoi(c.DefaultQuery("page", strconv.Itoa(options.DefaultPage)))
	pageSize, _ = strconv.Atoi(c.DefaultQuery("page_size", strconv.Itoa(options.DefaultPageSize)))
	if page < 1 {
		page = options.DefaultPage
	}
	if pageSize < 1 || pageSize > options.MaxPageSize {
		pageSize = options.DefaultPageSize
	}

	keyword = c.Query("keyword")
	return page, pageSize, keyword
}
