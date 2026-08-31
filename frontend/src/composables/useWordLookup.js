import { reactive } from 'vue'
import { apiGet, apiPost } from '@/api/client'

// 例句/近反义/形近词里点击单词时的「点击查询」逻辑，模块级单例：所有组件共享同一个 tooltip。
// 先精确查个人词库（GET /api/words/lookup），命中直接展示、不改动任何状态；
// 未命中则「自动录入一次」（POST /api/words），拿到该词信息后再展示。
// 录入后若还在查词中（translating），轮询直到释义写回再更新 tooltip 内容。
const state = reactive({
  open: false,
  x: 0,
  y: 0,
  word: '',
  wordKey: '',
  loading: false,
  error: '',
  data: null, // Word 对象（含 senses）
})

let pollTimer = null

function stopPoll() {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
}

// pollSenses 轮询某个查词中的单词，释义写回后更新 tooltip，否则继续等下一次。
async function pollSenses(id) {
  if (!state.open) {
    stopPoll()
    return
  }
  try {
    const fresh = await apiGet(`/api/words/translating?ids=${id}`)
    const w = Array.isArray(fresh) ? fresh[0] : null
    if (w && !w.translating) {
      state.data = w
      stopPoll()
    }
  } catch {
    // 轮询失败静默，等下一次再试
  }
}

async function resolve(word) {
  const existing = await apiGet(`/api/words/lookup?word=${encodeURIComponent(word)}`)
  if (existing) return existing
  return apiPost('/api/words', { word })
}

export function useWordLookup() {
  async function open(event, word) {
    stopPoll()
    const raw = String(word ?? '')
    state.word = raw
    state.wordKey = raw.toLowerCase()
    state.x = event.clientX
    state.y = event.clientY
    state.loading = true
    state.error = ''
    state.data = null
    state.open = true
    try {
      const data = await resolve(raw)
      state.data = data
      if (data && data.translating) {
        pollTimer = setInterval(() => pollSenses(data.id), 2000)
      }
    } catch (e) {
      state.error = e.message || '查询失败'
    } finally {
      state.loading = false
    }
  }

  function close() {
    stopPoll()
    state.open = false
    state.data = null
  }

  return { state, open, close }
}
