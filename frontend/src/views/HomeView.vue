<script setup>
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { apiGet, apiPost } from '@/api/client'
import { useWordActions } from '@/composables/useWordActions'
import { useTranslatingPoll } from '@/composables/useTranslatingPoll'
import LoginForm from '@/components/auth/LoginForm.vue'
import WordList from '@/components/word/WordList.vue'

const auth = useAuthStore()

const inputWord = ref('')
const wordInputRef = ref(null)
const words = ref([])
const submitting = ref(false)
const errorMsg = ref('')
const placeholderHint = '重复输入同一个单词会累加背诵次数'
const sortMode = ref(localStorage.getItem('vocab_sort_mode') || 'count')

const totalReviews = computed(() => words.value.reduce((sum, w) => sum + w.review_count, 0))

const { archiveWord, deleteWord } = useWordActions(words)
const { scheduleIfNeeded, stop } = useTranslatingPoll(words, loadWords)

function setSortMode(mode) {
  sortMode.value = mode
  localStorage.setItem('vocab_sort_mode', mode)
  sortWords()
}

function sortWords() {
  words.value.sort((a, b) => {
    if (sortMode.value === 'time') {
      return new Date(b.last_reviewed_at) - new Date(a.last_reviewed_at)
    }
    if (sortMode.value === 'alpha') {
      return a.word_key.localeCompare(b.word_key)
    }
    if (b.review_count !== a.review_count) return b.review_count - a.review_count
    return new Date(b.last_reviewed_at) - new Date(a.last_reviewed_at)
  })
}

async function loadWords() {
  try {
    words.value = await apiGet('/api/words')
    sortWords()
    scheduleIfNeeded()
  } catch {
    errorMsg.value = '列表加载失败，请刷新重试'
  }
}

async function submitWord() {
  const word = inputWord.value.trim()
  if (!word || submitting.value) return
  submitting.value = true
  errorMsg.value = ''
  try {
    const data = await apiPost('/api/words', { word })
    const idx = words.value.findIndex((w) => w.id === data.id)
    if (idx >= 0) {
      words.value[idx] = data
    } else {
      words.value.push(data)
    }
    sortWords()
    scheduleIfNeeded()
    inputWord.value = ''
  } catch (e) {
    errorMsg.value = e.message || '记录失败'
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
      loadWords()
    } else {
      words.value = []
      stop()
    }
  },
)

onMounted(() => {
  if (auth.isAuthenticated) loadWords()
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
      <p class="hint" :class="{ error: !!errorMsg }">{{ errorMsg || placeholderHint }}</p>

      <div class="stats" v-if="words.length">
        <span>共 {{ words.length }} 个单词 · 累计背诵 {{ totalReviews }} 次</span>
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
        @archive="archiveWord"
        @delete="deleteWord"
      />
    </div>
    <div class="footer">v1.5.0</div>
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
