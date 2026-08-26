import { ref } from 'vue'

export const PAGE_SIZE = 20

// 后端分页列表的共享状态机，供单词列表、归档列表、后台词库表复用。
// fetchPage(page, limit) 需要返回后端的分页信封 { items, total, has_more }。
export function usePaginatedList(fetchPage, { pageSize = PAGE_SIZE } = {}) {
  const items = ref([])
  const total = ref(0)
  // totalAll 为不受筛选影响的整表总数（仅部分接口返回），用于顶部展示全量计数
  const totalAll = ref(0)
  const loading = ref(false)
  const hasMore = ref(true)
  const loaded = ref(false) // 是否已经成功加载过第一页，用于区分「还没加载」和「确实是空列表」
  const errorMsg = ref('')

  let page = 0
  // reqSeq 给每次请求编号：切换排序或过滤条件时会重置并重新拉第一页，
  // 如果旧请求晚于新请求返回，会把新结果覆盖掉——按序号丢弃过期响应。
  let reqSeq = 0

  async function loadMore() {
    if (loading.value || !hasMore.value) return
    const seq = ++reqSeq
    const nextPage = page + 1
    loading.value = true
    errorMsg.value = ''
    try {
      const data = await fetchPage(nextPage, pageSize)
      if (seq !== reqSeq) return // 已有更新的请求发出，丢弃这次结果
      page = nextPage
      items.value = nextPage === 1 ? data.items : items.value.concat(data.items)
      total.value = data.total
      totalAll.value = data.total_all ?? 0
      hasMore.value = data.has_more
      loaded.value = true
    } catch (e) {
      if (seq !== reqSeq) return
      errorMsg.value = e.message || '加载失败，请重试'
    } finally {
      if (seq === reqSeq) loading.value = false
    }
  }

  // reset 回到第一页重新加载（排序、过滤条件变化时调用）
  async function reset() {
    reqSeq++ // 作废所有在途请求，避免旧的第 N 页数据追加到新条件的列表里
    page = 0
    items.value = []
    total.value = 0
    totalAll.value = 0
    hasMore.value = true
    loaded.value = false
    loading.value = false
    await loadMore()
  }

  // removeItem 本地删掉一条并同步总数，避免删除后计数与列表不一致
  function removeItem(predicate) {
    const before = items.value.length
    items.value = items.value.filter((x) => !predicate(x))
    if (items.value.length < before) {
      total.value = Math.max(0, total.value - (before - items.value.length))
    }
  }

  return { items, total, totalAll, loading, hasMore, loaded, errorMsg, reset, loadMore, removeItem }
}
