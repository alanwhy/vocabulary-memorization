<script setup>
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { apiGet, apiPost } from '@/api/client'
import { usePaginatedList } from '@/composables/usePaginatedList'
import { useWordActions } from '@/composables/useWordActions'
import { useTranslatingPoll } from '@/composables/useTranslatingPoll'
import { refreshVocabularyIndex } from '@/composables/useVocabularyIndex'
import LoginForm from '@/components/auth/LoginForm.vue'
import WordList from '@/components/word/WordList.vue'

const auth = useAuthStore()

const inputWord = ref('')
const wordInputRef = ref(null)
const submitting = ref(false)
const submitError = ref('')
const placeholderHint = '重复输入同一个单词会累加背诵次数'

// 三个排序字段及其默认方向；sortMode 存「字段 + 方向」组合值（默认方向不带后缀）
const SORT_FIELDS = [
  { key: 'count', label: '按次数', defaultDir: 'desc' },
  { key: 'time', label: '按时间', defaultDir: 'desc' },
  { key: 'alpha', label: '按字母', defaultDir: 'asc' },
]
const sortMode = ref(localStorage.getItem('vocab_sort_mode') || 'count')

// sortFieldOf / sortDirOf 从组合值里解析出当前字段和方向；未知值兜底回 count
function sortFieldOf(mode) {
  return (SORT_FIELDS.find((f) => mode === f.key || mode.startsWith(f.key + '_')) || SORT_FIELDS[0]).key
}
function sortDirOf(mode) {
  const field = SORT_FIELDS.find((f) => f.key === sortFieldOf(mode))
  if (mode.endsWith('_asc')) return 'asc'
  if (mode.endsWith('_desc')) return 'desc'
  return field.defaultDir
}

// 排序交给后端：分页后前端手里只有当前几页，本地排序会排出错误的顺序
const list = usePaginatedList((page, limit) =>
  apiGet(`/api/words?archived=0&sort=${sortMode.value}&page=${page}&limit=${limit}`),
)
const { items: words, loading, hasMore, loaded, errorMsg: listError, reset, loadMore } = list

// 顶部两个数字走 /api/stats，和列表 total 分开：total 只反映未归档的当前页条件，
// 而累计背诵次数必须由后端聚合，前端拿不到全量数据自己加。
const stats = ref({ total_words: 0, total_reviews: 0, today_reviews: 0 })

const hintText = computed(() => submitError.value || listError.value || placeholderHint)
const hasError = computed(() => !!(submitError.value || listError.value))

const { archiveWord, deleteWord, retryWord } = useWordActions(list, { onRemoved: dropFromStats })
const { scheduleIfNeeded, stop } = useTranslatingPoll(words, mergeTranslating)

function dropFromStats(w) {
  stats.value = {
    ...stats.value,
    total_words: Math.max(0, stats.value.total_words - 1),
    total_reviews: Math.max(0, stats.value.total_reviews - w.review_count),
    today_reviews: Math.max(0, (stats.value.today_reviews || 0) - w.review_count),
  }
}

// 重新查询成功后该词进入「查询中」，重启轮询直到后台把释义写回
async function handleRetry(w) {
  const ok = await retryWord(w)
  if (ok) scheduleIfNeeded()
}

async function loadStats() {
  try {
    stats.value = await apiGet('/api/stats')
  } catch {
    // 顶部只是两个数字，拉不到就保持旧值，不用打扰正在录入的用户
  }
}

// toggleSort 循环切换排序：点到别的字段就切到它的默认方向，点当前字段就反转方向
function toggleSort(fieldKey) {
  const field = SORT_FIELDS.find((f) => f.key === fieldKey)
  let next
  if (sortFieldOf(sortMode.value) === fieldKey) {
    const opposite = sortDirOf(sortMode.value) === 'desc' ? 'asc' : 'desc'
    next = opposite === field.defaultDir ? field.key : `${field.key}_${opposite}`
  } else {
    next = field.key
  }
  sortMode.value = next
  localStorage.setItem('vocab_sort_mode', next)
  reload()
}

async function reload() {
  await reset()
  scheduleIfNeeded()
}

// mergeTranslating 只把查词中的那几条 patch 回已加载的列表，不整表重载：
// 重载会把滚动加载出来的第 2、3…页全丢掉，滚动位置也跟着跳。
async function mergeTranslating() {
  const pendingIds = words.value.filter((w) => w.translating).map((w) => w.id)
  if (!pendingIds.length) {
    stop()
    return
  }
  let fresh
  try {
    fresh = await apiGet(`/api/words/translating?ids=${pendingIds.join(',')}`)
  } catch {
    return // 轮询失败就等下一次，不打断页面
  }
  const byId = new Map(fresh.map((w) => [w.id, w]))
  words.value = words.value.map((w) => byId.get(w.id) || w)
  scheduleIfNeeded()
}

async function submitWord() {
  const word = inputWord.value.trim()
  if (!word || submitting.value) return
  submitting.value = true
  submitError.value = ''
  try {
    const data = await apiPost('/api/words', { word })
    const idx = words.value.findIndex((w) => w.id === data.id)
    if (idx >= 0) {
      // 已存在：累加次数 + 更新最近背诵时间后，顺序在「按次数」「按时间」两种排序下都会变，
      // 但只有首页前几条能直观反映这一变化。如果不在第一页，直接 reload 让后端重新排序；
      // 在第一页的话就地调整：按时间排则挪到最顶，按次数排则按当前次数插到正确位置。
      const field = sortFieldOf(sortMode.value)
      const dir = sortDirOf(sortMode.value)
      if (field === 'time' && dir === 'desc') {
        words.value.splice(idx, 1)
        words.value.unshift(data)
      } else if (field === 'count' && dir === 'desc') {
        words.value.splice(idx, 1)
        const insertAt = words.value.findIndex((w) => w.review_count < data.review_count)
        words.value.splice(insertAt === -1 ? words.value.length : insertAt, 0, data)
      } else if (field === 'alpha') {
        // 字母排序：次数变化不影响顺序，方向也不影响，原地替换即可
        words.value[idx] = data
      } else {
        // 反向的按次数/按时间：就地插入的方向逻辑相反，交给后端重新排序更稳妥
        await reload()
      }
    } else {
      // 不在已加载的页里（新词，或排在后面的页）：插到最前面，让用户马上看见刚录入的词
      words.value.unshift(data)
    }
    inputWord.value = ''
    scheduleIfNeeded()
    loadStats()
    // 录入（或再次录入）后词库发生变化，刷新索引让高亮随新词/次数实时更新
    refreshVocabularyIndex()
  } catch (e) {
    submitError.value = e.message || '记录失败'
  } finally {
    submitting.value = false
    await nextTick()
    wordInputRef.value?.focus()
  }
}

watch(
  () => auth.isAuthenticated,
  (isAuth) => {
    if (isAuth) {
      reload()
      loadStats()
    } else {
      words.value = []
      stop()
    }
  },
)

onMounted(() => {
  if (auth.isAuthenticated) {
    reload()
    loadStats()
    nextTick(() => wordInputRef.value?.focus())
  }
})
</script>

<template>
  <div>
    <LoginForm v-if="!auth.isAuthenticated" />
    <div v-else>
      <h1>背单词</h1>
      <div class="input-wrap">
        <el-input
          ref="wordInputRef"
          v-model="inputWord"
          :disabled="submitting"
          placeholder="输入英文单词，按回车记录"
          @keyup.enter="submitWord"
        />
        <el-button type="primary" :disabled="submitting" @click="submitWord">添加</el-button>
      </div>
      <p class="hint" :class="{ error: hasError }">{{ hintText }}</p>

      <div class="stats" v-if="words.length">
        <span>共 {{ stats.total_words }} 个单词 · 累计背诵 {{ stats.total_reviews }} 次 · 今日 {{ stats.today_reviews || 0 }} 次</span>
        <span class="sort-toggle">
          <button
            v-for="f in SORT_FIELDS"
            :key="f.key"
            :class="{ active: sortFieldOf(sortMode) === f.key }"
            @click="toggleSort(f.key)"
          >
            {{ f.label }}
            <span v-if="sortFieldOf(sortMode) === f.key" class="arrow">
              {{ sortDirOf(sortMode) === 'desc' ? '▼' : '▲' }}
            </span>
          </button>
        </span>
      </div>

      <WordList
        :words="words"
        mode="active"
        empty-text="还没有记录，输入一个单词试试"
        :loading="loading || !loaded"
        :has-more="hasMore"
        @archive="archiveWord"
        @delete="deleteWord"
        @retry="handleRetry"
        @load-more="loadMore"
      />
    </div>
    <div class="footer">v1.16.1</div>
  </div>
</template>

<style scoped>
.input-wrap {
  margin-bottom: 8px;
  display: flex;
  gap: 8px;
  align-items: center;
}
.input-wrap :deep(.el-input) {
  flex: 1;
}
.hint {
  font-size: 13px;
  color: var(--muted);
  margin: 6px 4px 24px;
  min-height: 16px;
}
.hint.error {
  color: var(--danger);
}
.stats {
  font-size: 13px;
  color: var(--muted);
  margin: 0 4px 12px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 8px;
}
.sort-toggle {
  display: flex;
  gap: 4px;
}
.sort-toggle button {
  font-size: 12px;
  color: var(--muted);
  background: transparent;
  border: 1px solid var(--border);
  border-radius: 6px;
  padding: 3px 8px;
  cursor: pointer;
}
.sort-toggle button.active {
  color: var(--accent);
  border-color: var(--accent);
  background: var(--accent-soft);
}
.sort-toggle .arrow {
  margin-left: 2px;
  font-size: 10px;
}
</style>
