/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

import request from '@/utils/request'

export function getWorkspaces(params) {
  return request({
    url: '/workspaces',
    method: 'get',
    params
  })
}

export function createWorkspace(data) {
  return request({
    url: '/workspaces',
    method: 'post',
    data
  })
}

export function updateWorkspaceQuota(workspaceId, quotaBytes) {
  return request.put(`/workspaces/${workspaceId}`, { quota_bytes: quotaBytes })
}

export function addWorkspaceMember(workspaceId, data) {
  return request({
    url: `/workspaces/${workspaceId}/members`,
    method: 'post',
    data
  })
}

export function getWorkspaceMembers(workspaceId, params) {
  return request({
    url: `/workspaces/${workspaceId}/members`,
    method: 'get',
    params
  })
}

export function getWorkspaceAvailableUsers(workspaceId, params) {
  return request({
    url: `/workspaces/${workspaceId}/available-users`,
    method: 'get',
    params
  })
}

export function removeWorkspaceMember(workspaceId, userId) {
  return request({
    url: `/workspaces/${workspaceId}/members/${userId}`,
    method: 'delete'
  })
}

export function updateWorkspaceMemberQuota(workspaceId, userId, quotaBytes) {
  return request.put(`/workspaces/${workspaceId}/members/${userId}/quota`, { quota_bytes: quotaBytes })
}

export function getWorkspaceGroups(workspaceId, params) {
  return request({
    url: `/workspaces/${workspaceId}/groups`,
    method: 'get',
    params
  })
}

export function getWorkspaceGroupDirectory(workspaceId) {
  return request({
    url: `/workspaces/${workspaceId}/groups/directory`,
    method: 'get'
  })
}

export function createWorkspaceGroup(workspaceId, data) {
  return request({
    url: `/workspaces/${workspaceId}/groups`,
    method: 'post',
    data
  })
}

export function updateWorkspaceGroup(workspaceId, groupId, data) {
  return request({
    url: `/workspaces/${workspaceId}/groups/${groupId}`,
    method: 'put',
    data
  })
}

export function deleteWorkspaceGroup(workspaceId, groupId) {
  return request({
    url: `/workspaces/${workspaceId}/groups/${groupId}`,
    method: 'delete'
  })
}

export function addGroupMember(workspaceId, groupId, data) {
  return request({
    url: `/workspaces/${workspaceId}/groups/${groupId}/members`,
    method: 'post',
    data
  })
}

export function getGroupMembers(workspaceId, groupId, params) {
  return request({
    url: `/workspaces/${workspaceId}/groups/${groupId}/members`,
    method: 'get',
    params
  })
}

export function removeGroupMember(workspaceId, groupId, userId) {
  return request({
    url: `/workspaces/${workspaceId}/groups/${groupId}/members/${userId}`,
    method: 'delete'
  })
}
