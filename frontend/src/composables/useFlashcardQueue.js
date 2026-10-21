import { ref, computed } from 'vue'
import { apiGet, apiPost } from '@/api/client'

// 闪卡队列进度模块级单例：切到其它页面再切回闪卡页时沿用同一份队列和背卡进度，
// 不因组件重新挂载而重新拉取队列、把进度计数冲回第一张。
const cards = ref([])
const index = ref(0)
const flipped = ref(false)
const submitting = ref(false)
const loading = ref(false)
const error = ref('')
const doneCount = ref(0)
// 与队列进度一起保留本组成功评分，切出页面再回来仍能查看结果。
const groupResults = ref([])
let loaded = false

const current = computed(() => cards.value[index.value] ?? null)
// 当前组是否已背完（含空组）
const finished = computed(() => !loading.value && loaded && index.value >= cards.value.length)
// 真正没有待复习单词（组为空）
const allDone = computed(() => finished.value && cards.value.length === 0)
const groupCounts = computed(() => ({
  again: groupResults.value.filter((result) => result.rating === 'again').length,
  fuzzy: groupResults.value.filter((result) => result.rating === 'fuzzy').length,
  good: groupResults.value.filter((result) => result.rating === 'good').length,
}))

// 翻转过渡时长（ms），需与 .flip-inner 的 transition 时长保持一致
const FLIP_MS = 500

async function loadQueue() {
  if (loading.value || submitting.value) return
  loading.value = true
  error.value = ''
  try {
    cards.value = await apiGet('/api/flashcards/queue')
    index.value = 0
    flipped.value = false
    // 只有下一组加载成功才清空结果，加载失败时仍可回顾上一组并重试。
    groupResults.value = []
    loaded = true
  } catch (e) {
    error.value = e.message || '加载失败'
  } finally {
    loading.value = false
  }
}

// 只在从未加载过队列时才拉取，进入闪卡页时调用；已有队列直接沿用，保留背卡进度
function ensureQueue() {
  if (loaded || loading.value) return
  loadQueue()
}

function flip() {
  if (!current.value || loading.value || submitting.value) return
  flipped.value = !flipped.value
}

async function rate(rating) {
  if (!current.value || loading.value || submitting.value) return
  const word = current.value
  if (word.translating) return
  submitting.value = true
  error.value = ''
  try {
    await apiPost('/api/flashcards/review', { id: word.id, rating })
    // 按本组实际选择统计，不使用历史背诵次数；失败的评分不记入结果。
    groupResults.value.push({ word, rating })
    doneCount.value += 1
    if (flipped.value) {
      // 先翻回正面（仍是当前单词），等翻转动画结束再切下一张，
      // 避免翻转回正面的动画期间露出下一张卡片的背面中文释义
      flipped.value = false
      await new Promise((resolve) => setTimeout(resolve, FLIP_MS))
    }
    index.value += 1
  } catch (e) {
    error.value = e.message || '提交失败'
  } finally {
    submitting.value = false
  }
}

// 本地内容版本防止较早的轮询响应覆盖已成功纠错的新内容。
const contentVersions = new Map()
// 仅合并内容字段，后台补查/纠错不改变队列位置、评分结果或朗读触发。
function updateContent(fresh) {
  const word = cards.value.find((w) => w.id === fresh.id)
  if (word) {
    contentVersions.set(fresh.id, (contentVersions.get(fresh.id) || 0) + 1)
    Object.assign(word, { senses: fresh.senses, translating: fresh.translating })
  }
}
let refreshing = false
async function refreshContent() {
  if (refreshing) return
  const pending = cards.value.filter((w) => w.translating)
  if (!pending.length) return
  const versions = new Map(pending.map((w) => [w.id, contentVersions.get(w.id) || 0]))
  refreshing = true
  try {
    const fresh = await apiGet(`/api/words/translating?ids=${pending.map((w) => w.id).join(',')}`)
    for (const word of fresh) {
      if (versions.get(word.id) === (contentVersions.get(word.id) || 0)) updateContent(word)
    }
  } catch (e) {
    error.value = e.message || '补全状态查询失败，请重试'
  } finally {
    refreshing = false
  }
}

export function useFlashcardQueue() {
  return {
    cards,
    index,
    flipped,
    submitting,
    loading,
    error,
    doneCount,
    groupResults,
    groupCounts,
    current,
    finished,
    allDone,
    loadQueue,
    ensureQueue,
    flip,
    rate,
    refreshContent,
    updateContent,
  }
}
