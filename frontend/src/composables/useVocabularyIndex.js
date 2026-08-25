import { reactive } from 'vue'
import { apiGet } from '@/api/client'

// 全局词库索引（word_key -> occurrence_count），模块级单例：所有组件共享一份，
// ensure 首次拉取后缓存；录入/再次录入后调用 refresh 重新拉取，
// 让已加载卡片里「命中词库的词」的高亮随新词/次数变化实时刷新。
const state = reactive({ map: new Map(), loaded: false })
let loading = false

async function fetchIndex() {
  const items = await apiGet('/api/vocabulary')
  const m = new Map()
  for (const it of items) m.set(it.word_key, it.occurrence_count)
  state.map = m
  state.loaded = true
}

export async function ensureVocabularyIndex() {
  if (state.loaded || loading) return
  loading = true
  try {
    await fetchIndex()
  } catch {
    // 加载失败静默降级：不高亮，不影响单词展示
  } finally {
    loading = false
  }
}

// refresh 与 ensure 不同：已加载过也会重新拉取，用于录入后同步词库变化
export async function refreshVocabularyIndex() {
  if (loading) return
  loading = true
  try {
    await fetchIndex()
  } catch {
    // 刷新失败保留旧索引，不影响展示
  } finally {
    loading = false
  }
}

export function useVocabularyIndex() {
  return {
    // lookup 返回词在全局词库的出现次数，0 表示不在词库（不命中）
    lookup: (word) => (state.map.get(word) || 0),
    ensure: ensureVocabularyIndex,
    refresh: refreshVocabularyIndex,
  }
}
