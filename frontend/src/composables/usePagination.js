/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

import { ref } from 'vue'

/**
 * 通用分页组合式 API (Composable)
 * @param {Function} fetchMethod - 调用的后端 API 方法，需接受参数 { page, page_size, keyword }
 * @param {Object} defaultOptions - 默认配置选项 (可选)
 */
export function usePagination(fetchMethod, defaultOptions = {}) {
  const list = ref([])
  const loading = ref(false)
  const page = ref(defaultOptions.page || 1)
  const pageSize = ref(defaultOptions.pageSize || 20)
  const total = ref(0)
  const keyword = ref('')
  const error = ref(null)

  const load = async (extraParams = {}) => {
    loading.value = true
    error.value = null
    try {
      const res = await fetchMethod({ 
        page: page.value, 
        page_size: pageSize.value, 
        keyword: keyword.value, 
        ...extraParams 
      })
      
      if (res.data) {
        if (res.data.list !== undefined) {
          list.value = res.data.list || []
          total.value = res.data.total || 0
        } else {
          list.value = Array.isArray(res.data) ? res.data : []
          total.value = list.value.length
        }
      }
    } catch (loadError) {
      error.value = loadError
      list.value = []
      total.value = 0
    } finally {
      loading.value = false
    }
  }

  const handleSearch = () => {
    page.value = 1
    load()
  }

  const handleSizeChange = () => {
    page.value = 1
    load()
  }

  return {
    list,
    loading,
    page,
    pageSize,
    total,
    keyword,
    error,
    load,
    handleSearch,
    handleSizeChange
  }
}
