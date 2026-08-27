import { ElMessage, ElMessageBox } from 'element-plus'
import { apiPost, apiDelete } from '@/api/client'

// 归档/取消归档/删除单词的共享逻辑，供 HomeView 和 ArchiveView 复用，避免逐字复制。
// 入参 list 是 usePaginatedList 返回的对象：走它的 removeItem 而不是自己过滤 items，
// 这样列表和 total 一起更新，不会出现「删掉一条但总数没变」。
// onRemoved 给调用方一个机会同步别处的数字（首页顶部的统计是独立接口拿的，得自己减）。
export function useWordActions(list, { onRemoved } = {}) {
  function remove(w) {
    list.removeItem((x) => x.id === w.id)
    onRemoved?.(w)
  }

  async function archiveWord(w) {
    try {
      await apiPost(`/api/words/${w.id}/archive`)
      remove(w)
    } catch {
      ElMessage.error('归档失败，请重试')
    }
  }

  async function unarchiveWord(w) {
    try {
      await apiPost(`/api/words/${w.id}/unarchive`)
      remove(w)
    } catch {
      ElMessage.error('操作失败，请重试')
    }
  }

  async function deleteWord(w) {
    try {
      await ElMessageBox.confirm(`确定删除「${w.display_word}」吗？`, '提示', {
        confirmButtonText: '删除',
        cancelButtonText: '取消',
        type: 'warning',
      })
    } catch {
      return
    }
    try {
      await apiDelete(`/api/words/${w.id}`)
      remove(w)
    } catch {
      ElMessage.error('删除失败，请重试')
    }
  }

  // 对「无释义 / 查询失败 / 拼写错误」的词重新触发一次查词。
  // 成功就地把该词标成查询中、清掉错误占位，返回 true 让调用方重启轮询。
  async function retryWord(w) {
    try {
      await apiPost(`/api/words/${w.id}/retry`)
      w.translating = true
      w.senses = (w.senses || []).filter((s) => s.pos !== 'error')
      return true
    } catch {
      ElMessage.error('重新查询失败，请重试')
      return false
    }
  }

  return { archiveWord, unarchiveWord, deleteWord, retryWord }
}
