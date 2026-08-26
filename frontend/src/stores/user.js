/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

import { defineStore } from 'pinia'
import { getSession, login as loginApi, logout as logoutApi, switchWorkspace as switchWorkspaceApi } from '../api/auth'

let sessionPromise = null

export const useUserStore = defineStore('user', {
  state: () => ({
    user: null,
    menus: [],
    permissions: [],
    workspaces: [],
    currentWorkspaceId: null,
    currentWorkspaceCrossAccess: false,
    crossWorkspaceReason: '',
    sessionStatus: 'idle'
  }),

  getters: {
    isLoggedIn: (state) => state.sessionStatus === 'ready' && !!state.user,
    hasPermission: (state) => (permission) => {
      if (state.permissions.includes('*')) return true
      return state.permissions.includes(permission)
    }
  },

  actions: {
    async login(username, password) {
      const res = await loginApi({ username, password })
      this.applySession(res.data)
      return res
    },

    applySession(session) {
      this.user = session.user
      this.menus = session.menus || []
      this.permissions = session.permissions || []
      this.workspaces = session.workspaces || []
      this.currentWorkspaceId = session.current_workspace_id || null
      this.currentWorkspaceCrossAccess = Boolean(session.current_workspace_cross_access)
      this.crossWorkspaceReason = session.cross_workspace_reason || ''
      this.sessionStatus = 'ready'
    },

    applyProfile(profile) {
      if (!this.user || !profile) return
      this.user = {
        ...this.user,
        real_name: profile.real_name,
        email: profile.email,
        phone: profile.phone,
        source: profile.source || this.user.source
      }
    },

    async ensureSession() {
      if (this.sessionStatus === 'ready') return
      if (sessionPromise) return sessionPromise

      this.sessionStatus = 'loading'
      sessionPromise = getSession()
        .then(res => this.applySession(res.data))
        .catch(error => {
		  this.clearSession()
          throw error
        })
        .finally(() => {
          sessionPromise = null
        })
      return sessionPromise
    },

    invalidateSession() {
	  if (this.user) {
        this.sessionStatus = 'idle'
      }
    },

    clearSession() {
      this.user = null
      this.menus = []
      this.permissions = []
      this.workspaces = []
      this.currentWorkspaceId = null
      this.currentWorkspaceCrossAccess = false
      this.crossWorkspaceReason = ''
      this.sessionStatus = 'idle'
      sessionPromise = null
    },

    async logout() {
      try {
        if (this.user) await logoutApi()
      } finally {
        this.clearSession()
      }
    },

    async switchWorkspace(workspaceId, reason = '') {
      const res = await switchWorkspaceApi(workspaceId, reason)
      this.applySession(res.data)
      return res
    }
  }
})
