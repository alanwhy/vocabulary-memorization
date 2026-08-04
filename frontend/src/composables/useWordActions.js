import { ElMessage, ElMessageBox } from 'element-plus'
import { apiPost, apiDelete } from '@/api/client'

// 归档/取消归档/删除单词的共享逻辑，供 HomeView 和 ArchiveView 复用，避免逐字复制。
export function useWordActions(words) {
  async function archiveWord(w) {
    try {
      await apiPost(`/api/words/${w.id}/archive`)
      words.value = words.value.filter((x) => x.id !== w.id)
    } catch {
      ElMessage.error('归档失败，请重试')
    }
  }

  async function unarchiveWord(w) {
    try {
      await apiPost(`/api/words/${w.id}/unarchive`)
      words.value = words.value.filter((x) => x.id !== w.id)
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
      words.value = words.value.filter((x) => x.id !== w.id)
    } catch {
      ElMessage.error('删除失败，请重试')
    }
  }

  return { archiveWord, unarchiveWord, deleteWord }
}
