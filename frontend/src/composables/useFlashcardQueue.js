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
let loaded = false

const current = computed(() => cards.value[index.value] ?? null)
// 当前组是否已背完（含空组）
const finished = computed(() => !loading.value && loaded && index.value >= cards.value.length)
// 真正没有待复习单词（组为空）
const allDone = computed(() => finished.value && cards.value.length === 0)

// 翻转过渡时长（ms），需与 .flip-inner 的 transition 时长保持一致
const FLIP_MS = 500

async function loadQueue() {
  loading.value = true
  error.value = ''
  try {
    cards.value = await apiGet('/api/flashcards/queue')
    index.value = 0
    flipped.value = false
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
  if (submitting.value) return
  flipped.value = !flipped.value
}

async function rate(rating) {
  if (!current.value || submitting.value) return
  submitting.value = true
  error.value = ''
  try {
    await apiPost('/api/flashcards/review', { id: current.value.id, rating })
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

export function useFlashcardQueue() {
  return {
    cards,
    index,
    flipped,
    submitting,
    loading,
    error,
    doneCount,
    current,
    finished,
    allDone,
    loadQueue,
    ensureQueue,
    flip,
    rate,
  }
}
