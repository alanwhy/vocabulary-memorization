<script setup>
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { apiGet, apiPost } from '@/api/client'
import { usePaginatedList } from '@/composables/usePaginatedList'
import { useWordActions } from '@/composables/useWordActions'
import { useTranslatingPoll } from '@/composables/useTranslatingPoll'
import LoginForm from '@/components/auth/LoginForm.vue'
import WordList from '@/components/word/WordList.vue'

const auth = useAuthStore()

const inputWord = ref('')
const wordInputRef = ref(null)
const submitting = ref(false)
const submitError = ref('')
const placeholderHint = '重复输入同一个单词会累加背诵次数'
const sortMode = ref(localStorage.getItem('vocab_sort_mode') || 'count')

// 排序交给后端：分页后前端手里只有当前几页，本地排序会排出错误的顺序
const list = usePaginatedList((page, limit) =>
  apiGet(`/api/words?archived=0&sort=${sortMode.value}&page=${page}&limit=${limit}`),
)
const { items: words, loading, hasMore, loaded, errorMsg: listError, reset, loadMore } = list

// 顶部两个数字走 /api/stats，和列表 total 分开：total 只反映未归档的当前页条件，
// 而累计背诵次数必须由后端聚合，前端拿不到全量数据自己加。
const stats = ref({ total_words: 0, total_reviews: 0 })

const hintText = computed(() => submitError.value || listError.value || placeholderHint)
const hasError = computed(() => !!(submitError.value || listError.value))

const { archiveWord, deleteWord } = useWordActions(list, { onRemoved: dropFromStats })
const { scheduleIfNeeded, stop } = useTranslatingPoll(words, mergeTranslating)

function dropFromStats(w) {
  stats.value = {
    ...stats.value,
    total_words: Math.max(0, stats.value.total_words - 1),
    total_reviews: Math.max(0, stats.value.total_reviews - w.review_count),
  }
}

async function loadStats() {
  try {
    stats.value = await apiGet('/api/stats')
  } catch {
    // 顶部只是两个数字，拉不到就保持旧值，不用打扰正在录入的用户
  }
}

function setSortMode(mode) {
  if (mode === sortMode.value) return
  sortMode.value = mode
  localStorage.setItem('vocab_sort_mode', mode)
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
      // 已在当前页面上：原地替换，次数和释义立刻更新，顺序留到下次重新加载时再纠正
      words.value[idx] = data
    } else {
      // 不在已加载的页里（新词，或排在后面的页）：插到最前面，让用户马上看见刚录入的词
      words.value = [data, ...words.value]
    }
    inputWord.value = ''
    scheduleIfNeeded()
    loadStats()
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
  }
})
</script>

<template>
  <div>
    <LoginForm v-if="!auth.isAuthenticated" />
    <div v-else>
      <h1>背单词</h1>
      <div class="input-wrap">
        <input
          ref="wordInputRef"
          type="text"
          v-model="inputWord"
          :disabled="submitting"
          @keyup.enter="submitWord"
          placeholder="输入英文单词，按回车记录"
          autofocus
        />
        <button class="add-btn" :disabled="submitting" @click="submitWord">添加</button>
      </div>
      <p class="hint" :class="{ error: hasError }">{{ hintText }}</p>

      <div class="stats" v-if="words.length">
        <span>共 {{ stats.total_words }} 个单词 · 累计背诵 {{ stats.total_reviews }} 次</span>
        <span class="sort-toggle">
          <button :class="{ active: sortMode === 'count' }" @click="setSortMode('count')">按次数</button>
          <button :class="{ active: sortMode === 'time' }" @click="setSortMode('time')">按时间</button>
          <button :class="{ active: sortMode === 'alpha' }" @click="setSortMode('alpha')">按字母</button>
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
        @load-more="loadMore"
      />
    </div>
    <div class="footer">v1.7.0</div>
  </div>
</template>

<style scoped>
.input-wrap {
  margin-bottom: 8px;
  display: flex;
  gap: 8px;
}
input[type='text'] {
  width: 100%;
  padding: 14px 16px;
  font-size: 16px;
  border: 1px solid var(--border);
  border-radius: 10px;
  background: var(--card-bg);
  color: var(--text);
  outline: none;
  transition: border-color 0.15s ease;
}
input[type='text']:focus {
  border-color: var(--accent);
}
.add-btn {
  flex-shrink: 0;
  padding: 0 20px;
  font-size: 15px;
  border: none;
  border-radius: 10px;
  background: var(--accent);
  color: #fff;
  cursor: pointer;
}
.add-btn:disabled {
  opacity: 0.6;
  cursor: default;
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
</style>
