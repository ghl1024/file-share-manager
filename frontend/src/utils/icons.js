/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

import { Bell, Collection, Connection, DataBoard, Document, Folder, FolderOpened, Lock, Menu, Monitor, Setting, Share, User } from '@element-plus/icons-vue'

export const appIcons = {
  Bell,
  DataBoard,
  Collection,
  Connection,
  Document,
  Folder,
  FolderOpened,
  Lock,
  Menu,
  Monitor,
  Share,
  Setting,
  User
}

export function resolveAppIcon(name, fallback = 'Document') {
  return appIcons[name] || appIcons[fallback] || Document
}
