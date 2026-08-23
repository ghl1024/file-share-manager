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
	"fmt"
	"slices"
	"strconv"
	"strings"

	"file-share-manager/server/internal/model"
	"file-share-manager/server/internal/pkg/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type MenuDAO struct {
	db *gorm.DB
}

type MenuDefinition struct {
	Code        string
	ParentCode  string
	Name        string
	Path        string
	Component   string
	Type        int8
	Icon        string
	SortOrder   int
	Hidden      bool
	Status      int
	Permissions []string
}

var BuiltinMenus = []MenuDefinition{
	{Code: "dashboard", Name: "工作台", Path: "/dashboard", Component: "dashboard/Dashboard", Type: 1, Icon: "DataBoard", SortOrder: 10, Status: 1},
	{Code: "workspaces", Name: "工作空间", Path: "/workspaces", Component: "workspace/WorkspaceList", Type: 1, Icon: "FolderOpened", SortOrder: 20, Status: 1},
	{Code: "files", Name: "文件目录", Path: "/files", Component: "files/Files", Type: 1, Icon: "Folder", SortOrder: 30, Status: 1, Permissions: []string{"file:list"}},
	{Code: "files-upload", ParentCode: "files", Name: "上传文件", Type: 2, SortOrder: 31, Hidden: true, Status: 1, Permissions: []string{"file:upload"}},
	{Code: "files-download", ParentCode: "files", Name: "下载文件", Type: 2, SortOrder: 32, Hidden: true, Status: 1, Permissions: []string{"file:download"}},
	{Code: "files-delete", ParentCode: "files", Name: "删除文件", Type: 2, SortOrder: 33, Hidden: true, Status: 1, Permissions: []string{"file:delete"}},
	{Code: "files-restore", ParentCode: "files", Name: "恢复文件", Type: 2, SortOrder: 34, Hidden: true, Status: 1, Permissions: []string{"file:restore"}},
	{Code: "files-acl", ParentCode: "files", Name: "目录权限", Type: 2, SortOrder: 35, Hidden: true, Status: 1, Permissions: []string{"acl:manage"}},
	{Code: "shares", Name: "外链分享", Path: "/shares", Component: "share/ShareList", Type: 1, Icon: "Share", SortOrder: 40, Status: 1, Permissions: []string{"file:share:create"}},
	{Code: "audit", Name: "审计中心", Path: "/audit/history", Component: "audit/OperationHistory", Type: 1, Icon: "Document", SortOrder: 50, Status: 1, Permissions: []string{"audit:list"}},
	{Code: "audit-export", ParentCode: "audit", Name: "导出审计", Type: 2, SortOrder: 51, Hidden: true, Status: 1, Permissions: []string{"audit:export"}},
	{Code: "audit-archive", ParentCode: "audit", Name: "归档审计", Type: 2, SortOrder: 52, Hidden: true, Status: 1, Permissions: []string{"audit:archive"}},
	{Code: "system", Name: "系统管理", Path: "/system", Type: 0, Icon: "Setting", SortOrder: 90, Status: 1},
	{Code: "system-users", ParentCode: "system", Name: "用户管理", Path: "/system/user", Component: "system/UserManage", Type: 1, Icon: "User", SortOrder: 91, Status: 1},
	{Code: "system-menu", ParentCode: "system", Name: "菜单权限", Path: "/system/menu", Component: "system/MenuManage", Type: 1, Icon: "Menu", SortOrder: 92, Status: 1},
	{Code: "system-ldap", ParentCode: "system", Name: "LDAP", Path: "/system/ldap", Component: "system/SystemConfig", Type: 1, Icon: "Connection", SortOrder: 93, Status: 1},
	{Code: "system-clamav", ParentCode: "system", Name: "ClamAV", Path: "/system/clamav", Component: "system/SystemConfig", Type: 1, Icon: "Monitor", SortOrder: 94, Status: 1},
	{Code: "system-backup-storage", ParentCode: "system", Name: "备份存储", Path: "/system/backup-storage", Component: "system/SystemConfig", Type: 1, Icon: "Collection", SortOrder: 95, Status: 1},
	{Code: "system-notifications", ParentCode: "system", Name: "通知告警", Path: "/system/notifications", Component: "system/NotificationManage", Type: 1, Icon: "Bell", SortOrder: 96, Status: 1},
	{Code: "roles", ParentCode: "system", Name: "角色管理", Path: "/system/role", Component: "system/RoleManage", Type: 1, Icon: "Lock", SortOrder: 97, Status: 1, Permissions: []string{"workspace:role:manage"}},
	{Code: "backups", ParentCode: "system", Name: "备份恢复", Path: "/system/backups", Component: "system/BackupList", Type: 1, Icon: "Collection", SortOrder: 98, Status: 1, Permissions: []string{"backup:manage"}},
}

// builtinMenuNames is also used when reading legacy rows that were created by
// older versions with an empty or placeholder display name.
var builtinMenuNames = func() map[string]string {
	result := make(map[string]string, len(BuiltinMenus))
	for _, definition := range BuiltinMenus {
		result[definition.Code] = definition.Name
	}
	return result
}()

var builtinMenuNamesByPermission = func() map[string]string {
	result := make(map[string]string)
	for _, definition := range BuiltinMenus {
		for _, permission := range definition.Permissions {
			result[permission] = definition.Name
		}
	}
	return result
}()

func NewMenuDAO() *MenuDAO {
	return &MenuDAO{db: database.DB}
}

func (dao *MenuDAO) EnsureBuiltinMenus() error {
	return dao.db.Transaction(func(tx *gorm.DB) error {
		codeToID := map[string]uint{}
		for _, definition := range BuiltinMenus {
			parentID := uint(0)
			if definition.ParentCode != "" {
				parentID = codeToID[definition.ParentCode]
				if parentID == 0 {
					var parent model.Menu
					if err := tx.Where("code = ?", definition.ParentCode).First(&parent).Error; err != nil {
						return fmt.Errorf("parent menu %s not found: %w", definition.ParentCode, err)
					}
					parentID = parent.ID
				}
			}

			menu := model.Menu{
				Code: definition.Code, ParentID: parentID, Name: definition.Name, Path: definition.Path,
				Component: definition.Component, Type: definition.Type, Icon: definition.Icon,
				SortOrder: definition.SortOrder, Hidden: definition.Hidden, Status: definition.Status,
			}
			if menu.Status == 0 {
				menu.Status = 1
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "code"}},
				DoUpdates: clause.AssignmentColumns([]string{
					"parent_id", "name", "path", "component", "type", "icon", "sort_order", "hidden", "status",
				}),
			}).Create(&menu).Error; err != nil {
				return err
			}
			if menu.ID == 0 {
				if err := tx.Where("code = ?", definition.Code).First(&menu).Error; err != nil {
					return err
				}
			}
			codeToID[definition.Code] = menu.ID
			if err := replaceMenuPermissions(tx, menu.ID, normalizePermissionCodes(definition.Permissions)); err != nil {
				return err
			}
		}
		return repairLegacyBuiltinMenuNames(tx)
	})
}

func (dao *MenuDAO) ListTree() ([]model.Menu, error) {
	menus, err := dao.listAll()
	if err != nil {
		return nil, err
	}
	if err := dao.attachPermissions(menus); err != nil {
		return nil, err
	}
	repairBuiltinMenuNames(menus)
	return buildMenuTree(menus, 0), nil
}

func (dao *MenuDAO) ListTreeForAccess(isSuperAdmin, hasWorkspace bool, permissionSet map[string]bool) ([]model.Menu, error) {
	menus, err := dao.listAll()
	if err != nil {
		return nil, err
	}
	if len(menus) == 0 {
		return []model.Menu{}, nil
	}
	if err := dao.attachPermissions(menus); err != nil {
		return nil, err
	}
	repairBuiltinMenuNames(menus)
	visible := make([]model.Menu, 0, len(menus))
	for _, menu := range menus {
		if menu.Status != 1 || menu.Hidden || menu.Type == 2 {
			continue
		}
		if !hasWorkspace && menuRequiresWorkspace(menu) {
			continue
		}
		if menuAccessible(menu, isSuperAdmin, permissionSet) {
			visible = append(visible, menu)
		}
	}
	visible = ensureMenuParents(visible, menus)
	return buildMenuTree(visible, 0), nil
}

func repairBuiltinMenuNames(menus []model.Menu) {
	for i := range menus {
		expected := builtinMenuName(menus[i])
		if expected == "" {
			continue
		}
		name := strings.TrimSpace(menus[i].Name)
		if name == "" || name == "..." || name == "…" {
			menus[i].Name = expected
		}
	}
}

func builtinMenuName(menu model.Menu) string {
	if expected := builtinMenuNames[menu.Code]; expected != "" {
		return expected
	}
	for _, permission := range menu.Permissions {
		if expected := builtinMenuNamesByPermission[permission]; expected != "" {
			return expected
		}
	}
	return ""
}

func repairLegacyBuiltinMenuNames(tx *gorm.DB) error {
	var menus []model.Menu
	if err := tx.Where("name IS NULL OR TRIM(name) = '' OR name IN ?", []string{"...", "…"}).Find(&menus).Error; err != nil {
		return err
	}
	for _, menu := range menus {
		var permissions []string
		if err := tx.Model(&model.MenuPermission{}).Where("menu_id = ?", menu.ID).Pluck("permission_code", &permissions).Error; err != nil {
			return err
		}
		menu.Permissions = permissions
		if expected := builtinMenuName(menu); expected != "" {
			if err := tx.Model(&model.Menu{}).Where("id = ?", menu.ID).Update("name", expected).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func (dao *MenuDAO) Get(id uint) (*model.Menu, error) {
	var menu model.Menu
	err := dao.db.First(&menu, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	var permissions []string
	if err := dao.db.Model(&model.MenuPermission{}).Where("menu_id = ?", id).Order("permission_code ASC").Pluck("permission_code", &permissions).Error; err != nil {
		return nil, err
	}
	menu.Permissions = permissions
	return &menu, nil
}

func (dao *MenuDAO) Create(menu *model.Menu, permissions []string) error {
	return dao.CreateWithAudit(menu, permissions, nil)
}

func (dao *MenuDAO) CreateWithAudit(menu *model.Menu, permissions []string, event *model.OperationLog) error {
	return dao.db.Transaction(func(tx *gorm.DB) error {
		if err := validateMenuParent(tx, 0, menu.ParentID); err != nil {
			return err
		}
		if err := tx.Create(menu).Error; err != nil {
			return err
		}
		if err := replaceMenuPermissions(tx, menu.ID, normalizePermissionCodes(permissions)); err != nil {
			return err
		}
		prepareMenuAuditEvent(event, menu.ID, menu.Name)
		after, err := menuAuditSnapshot(tx, *menu)
		if err != nil {
			return err
		}
		return appendAuditEvent(tx, event, nil, after)
	})
}

func (dao *MenuDAO) Update(id uint, updates model.Menu, permissions []string) error {
	return dao.UpdateWithAudit(id, updates, permissions, nil)
}

func (dao *MenuDAO) UpdateWithAudit(id uint, updates model.Menu, permissions []string, event *model.OperationLog) error {
	return dao.db.Transaction(func(tx *gorm.DB) error {
		var beforeMenu model.Menu
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&beforeMenu, id).Error; err != nil {
			return err
		}
		before, err := menuAuditSnapshot(tx, beforeMenu)
		if err != nil {
			return err
		}
		if err := validateMenuParent(tx, id, updates.ParentID); err != nil {
			return err
		}
		result := tx.Model(&model.Menu{}).Where("id = ?", id).Updates(map[string]any{
			"parent_id": updates.ParentID, "name": updates.Name, "path": updates.Path,
			"component": updates.Component, "type": updates.Type, "icon": updates.Icon,
			"sort_order": updates.SortOrder, "hidden": updates.Hidden, "status": updates.Status,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		if err := replaceMenuPermissions(tx, id, normalizePermissionCodes(permissions)); err != nil {
			return err
		}
		var afterMenu model.Menu
		if err := tx.First(&afterMenu, id).Error; err != nil {
			return err
		}
		prepareMenuAuditEvent(event, id, afterMenu.Name)
		after, err := menuAuditSnapshot(tx, afterMenu)
		if err != nil {
			return err
		}
		return appendAuditEvent(tx, event, before, after)
	})
}

func (dao *MenuDAO) Delete(id uint) error {
	return dao.DeleteWithAudit(id, nil)
}

func (dao *MenuDAO) DeleteWithAudit(id uint, event *model.OperationLog) error {
	return dao.db.Transaction(func(tx *gorm.DB) error {
		var menu model.Menu
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&menu, id).Error; err != nil {
			return err
		}
		before, err := menuAuditSnapshot(tx, menu)
		if err != nil {
			return err
		}
		var children int64
		if err := tx.Model(&model.Menu{}).Where("parent_id = ?", id).Count(&children).Error; err != nil {
			return err
		}
		if children > 0 {
			return ErrMenuHasChildren
		}
		if err := tx.Where("menu_id = ?", id).Delete(&model.MenuPermission{}).Error; err != nil {
			return err
		}
		result := tx.Delete(&model.Menu{}, id)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		prepareMenuAuditEvent(event, id, menu.Name)
		return appendAuditEvent(tx, event, before, nil)
	})
}

func prepareMenuAuditEvent(event *model.OperationLog, menuID uint, name string) {
	if event == nil {
		return
	}
	event.TargetType = "menu"
	event.TargetID = strconv.FormatUint(uint64(menuID), 10)
	if event.TargetName == "" {
		event.TargetName = name
	}
}

func menuAuditSnapshot(tx *gorm.DB, menu model.Menu) (map[string]any, error) {
	var permissions []string
	if err := tx.Model(&model.MenuPermission{}).Where("menu_id = ?", menu.ID).
		Order("permission_code ASC").Pluck("permission_code", &permissions).Error; err != nil {
		return nil, err
	}
	return map[string]any{
		"id": menu.ID, "code": menu.Code, "parent_id": menu.ParentID, "name": menu.Name,
		"path": menu.Path, "component": menu.Component, "type": menu.Type, "icon": menu.Icon,
		"sort_order": menu.SortOrder, "hidden": menu.Hidden, "status": menu.Status,
		"permissions": permissions,
	}, nil
}

var ErrMenuHasChildren = errors.New("menu has children")

func (dao *MenuDAO) listAll() ([]model.Menu, error) {
	var menus []model.Menu
	if err := dao.db.Order("sort_order ASC, id ASC").Find(&menus).Error; err != nil {
		return nil, err
	}
	return menus, nil
}

func (dao *MenuDAO) attachPermissions(menus []model.Menu) error {
	if len(menus) == 0 {
		return nil
	}
	ids := make([]uint, 0, len(menus))
	for _, menu := range menus {
		ids = append(ids, menu.ID)
	}
	var links []model.MenuPermission
	if err := dao.db.Where("menu_id IN ?", ids).Order("permission_code ASC").Find(&links).Error; err != nil {
		return err
	}
	byMenu := map[uint][]string{}
	for _, link := range links {
		byMenu[link.MenuID] = append(byMenu[link.MenuID], link.PermissionCode)
	}
	for i := range menus {
		menus[i].Permissions = byMenu[menus[i].ID]
		if menus[i].Permissions == nil {
			menus[i].Permissions = []string{}
		}
	}
	return nil
}

func buildMenuTree(menus []model.Menu, parentID uint) []model.Menu {
	tree := make([]model.Menu, 0)
	for _, menu := range menus {
		if menu.ParentID == parentID {
			menu.Children = buildMenuTree(menus, menu.ID)
			tree = append(tree, menu)
		}
	}
	return tree
}

func ensureMenuParents(menus, all []model.Menu) []model.Menu {
	byID := make(map[uint]model.Menu, len(all))
	seen := make(map[uint]bool, len(menus))
	for _, menu := range all {
		byID[menu.ID] = menu
	}
	for _, menu := range menus {
		seen[menu.ID] = true
	}
	for {
		added := false
		for _, menu := range menus {
			if menu.ParentID == 0 || seen[menu.ParentID] {
				continue
			}
			parent, ok := byID[menu.ParentID]
			if !ok || parent.Status != 1 || parent.Hidden || parent.Type == 2 {
				continue
			}
			menus = append(menus, parent)
			seen[parent.ID] = true
			added = true
		}
		if !added {
			break
		}
	}
	slices.SortFunc(menus, func(a, b model.Menu) int {
		if a.SortOrder != b.SortOrder {
			return a.SortOrder - b.SortOrder
		}
		return int(a.ID) - int(b.ID)
	})
	return menus
}

func menuAccessible(menu model.Menu, isSuperAdmin bool, permissionSet map[string]bool) bool {
	if menu.Code == "dashboard" || menu.Code == "workspaces" {
		return true
	}
	if strings.HasPrefix(menu.Code, "system-") && menu.Code != "system" && menu.Code != "roles" && menu.Code != "backups" {
		return isSuperAdmin
	}
	if menu.Code == "system" {
		return false
	}
	if isSuperAdmin {
		return true
	}
	if len(menu.Permissions) == 0 {
		return true
	}
	for _, permission := range menu.Permissions {
		if permissionSet[permission] {
			return true
		}
	}
	return false
}

func menuRequiresWorkspace(menu model.Menu) bool {
	switch menu.Code {
	case "files", "shares", "audit", "roles", "backups":
		return true
	default:
		return false
	}
}

func validateMenuParent(tx *gorm.DB, selfID, parentID uint) error {
	if parentID == 0 {
		return nil
	}
	if selfID > 0 && parentID == selfID {
		return errors.New("父级菜单不能选择自身")
	}
	var parent model.Menu
	if err := tx.First(&parent, parentID).Error; err != nil {
		return err
	}
	if parent.Type == 2 {
		return errors.New("按钮不能作为父级")
	}
	if selfID == 0 {
		return nil
	}
	current := parent
	for current.ParentID != 0 {
		if current.ParentID == selfID {
			return errors.New("父级菜单不能选择自己的子级")
		}
		if err := tx.First(&current, current.ParentID).Error; err != nil {
			return err
		}
	}
	return nil
}

func replaceMenuPermissions(tx *gorm.DB, menuID uint, permissions []string) error {
	if err := tx.Where("menu_id = ?", menuID).Delete(&model.MenuPermission{}).Error; err != nil {
		return err
	}
	if len(permissions) == 0 {
		return nil
	}
	var definitions []model.Permission
	if err := tx.Where("code IN ?", permissions).Find(&definitions).Error; err != nil {
		return err
	}
	if len(definitions) != len(permissions) {
		return errors.New("菜单关联了不存在的权限点")
	}
	links := make([]model.MenuPermission, 0, len(permissions))
	for _, permission := range permissions {
		links = append(links, model.MenuPermission{MenuID: menuID, PermissionCode: permission})
	}
	return tx.Create(&links).Error
}

func normalizePermissionCodes(permissions []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(permissions))
	for _, permission := range permissions {
		permission = strings.TrimSpace(permission)
		if permission == "" {
			continue
		}
		if _, exists := seen[permission]; exists {
			continue
		}
		seen[permission] = struct{}{}
		result = append(result, permission)
	}
	slices.Sort(result)
	return result
}
