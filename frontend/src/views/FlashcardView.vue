<script setup>
import { ref, computed, onMounted } from 'vue'
import { apiGet, apiPost } from '@/api/client'
import { useVocabularyIndex } from '@/composables/useVocabularyIndex'
import { speakWord } from '@/composables/usePronunciation'
import { tokenizeExample, splitWordRef } from '@/utils/highlight'

const vocab = useVocabularyIndex()
const cards = ref([])
const index = ref(0)
const flipped = ref(false)
const submitting = ref(false)
const loading = ref(true)
const error = ref('')
const doneCount = ref(0)

const current = computed(() => cards.value[index.value] ?? null)
// 词级强化信息（音标/词根词缀/近反义）从第一条词性取（平铺模型下每条重复）
const firstSense = computed(() => (current.value?.senses && current.value.senses[0]) || {})
const phonetic = computed(() => firstSense.value.phonetic || '')
// 单词面例句：取第一条含英文例句的词性，仅展示英文原文（不含中文释义）
const frontExample = computed(() => (current.value?.senses || []).find((s) => s.example) || null)
const rootAffix = computed(() => {
  const parts = [firstSense.value.root, firstSense.value.affix].filter(Boolean)
  return parts.join(' + ') // 只返回词根词缀内容，「词根词缀：」标题由模板统一渲染
})
const enrichGroups = computed(() => {
  const mk = (label, list) => ({ label, refs: (list || []).map((r) => splitWordRef(r, vocab.lookup)) })
  return [
    mk('近义词', firstSense.value.synonyms),
    mk('反义词', firstSense.value.antonyms),
    mk('形近词', firstSense.value.lookalikes),
  ].filter((g) => g.refs.length)
})
const hasEnrichment = computed(() => !!(rootAffix.value || enrichGroups.value.length))

function hlClass(level) {
  return `hl-word hl-word--l${level}`
}

function exampleTokens(s) {
  return tokenizeExample(s.example || '', vocab.lookup)
}
// 当前组是否已背完（含空组）
const finished = computed(() => !loading.value && index.value >= cards.value.length)
// 真正没有待复习单词（组为空）
const allDone = computed(() => finished.value && cards.value.length === 0)
const progress = computed(() => `${Math.min(index.value + 1, cards.value.length)} / ${cards.value.length}`)

// 两个评分档位，与后端 good / again 对应；「记住」会直接归档
const RATINGS = [
  { key: 'again', label: '不认识', emoji: '❌', type: 'danger' },
  { key: 'good', label: '记住', emoji: '✅', type: 'success' },
]

async function loadQueue() {
  loading.value = true
  error.value = ''
  try {
    cards.value = await apiGet('/api/flashcards/queue')
    index.value = 0
    flipped.value = false
  } catch (e) {
    error.value = e.message || '加载失败'
  } finally {
    loading.value = false
  }
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
    index.value += 1
    flipped.value = false
  } catch (e) {
    error.value = e.message || '提交失败'
  } finally {
    submitting.value = false
  }
}

onMounted(() => {
  loadQueue()
  vocab.ensure()
})
</script>

<template>
  <main class="flashcards">
    <header class="head">
      <h2>闪卡复习</h2>
      <span v-if="cards.length" class="progress">{{ progress }}</span>
    </header>

    <div v-if="loading" class="state">加载中...</div>

    <div v-else-if="allDone" class="state done">
      <p>今日复习完成 🎉</p>
      <p class="sub">本次共复习 {{ doneCount }} 张卡片</p>
    </div>

    <div v-else-if="finished" class="state done">
      <p>本组完成 🎉</p>
      <p class="sub">本组背完 {{ cards.length }} 张，累计 {{ doneCount }} 张</p>
      <button class="reload-btn" @click="loadQueue">再来一组</button>
    </div>

    <template v-else>
      <div class="flip" :class="{ flipped }" @click="flip">
        <div class="flip-inner">
          <div class="face front">
            <span class="word">{{ current.display_word }}</span>
            <span class="phonetic" v-if="phonetic">{{ phonetic }}</span>
            <button
              type="button"
              class="speak-btn"
              :aria-label="`朗读 ${current.display_word}`"
              @click.stop="speakWord(current.word_key)"
            >
              🔊
            </button>
            <span class="front-example" v-if="frontExample">
              <span v-for="(t, i) in exampleTokens(frontExample)" :key="i" :class="t.level ? hlClass(t.level) : ''">{{ t.text }}</span>
            </span>
            <span class="hint">点击翻面看释义</span>
          </div>
          <div class="face back">
            <span class="phonetic" v-if="phonetic">{{ phonetic }}</span>
            <div class="senses" v-if="current.senses && current.senses.length">
              <div class="sense" v-for="(s, i) in current.senses" :key="i">
                <span class="pos" v-if="s.pos">{{ s.pos }}</span>
                <span class="translation">{{ s.translation }}</span>
                <span class="example" v-if="s.example || s.example_translation">
                  <span class="example-en" v-if="s.example">
                    <span v-for="(t, i) in exampleTokens(s)" :key="i" :class="t.level ? hlClass(t.level) : ''">{{ t.text }}</span>
                  </span>
                  <span class="example-trans" v-if="s.example_translation">{{ s.example_translation }}</span>
                </span>
              </div>
            </div>
            <div class="senses" v-else>
              <div class="sense">
                <span class="translation pending">{{ current.translating ? '查词中...' : '暂无释义' }}</span>
              </div>
            </div>
            <div class="enrich" v-if="hasEnrichment">
              <span v-if="rootAffix">
                <span class="enrich-label">词根词缀：</span>{{ rootAffix }}
              </span>
              <span v-for="g in enrichGroups" :key="g.label">
                <span class="enrich-label">{{ g.label }}：</span><span v-for="(r, i) in g.refs" :key="i">{{ i > 0 ? '、' : '' }}<span :class="r.level ? hlClass(r.level) : ''">{{ r.word }}</span>{{ r.rest }}</span>
              </span>
            </div>
            <span class="count">已背 ×{{ current.review_count }}</span>
          </div>
        </div>
      </div>

      <p v-if="error" class="error">{{ error }}</p>

      <div class="actions">
        <el-button
          v-for="r in RATINGS"
          :key="r.key"
          class="rate-btn"
          :type="r.type"
          size="large"
          :disabled="submitting"
          @click="rate(r.key)"
        >
          {{ r.emoji }} {{ r.label }}
        </el-button>
      </div>
    </template>
  </main>
</template>

<style scoped>
.flashcards {
  max-width: 520px;
  margin: 0 auto;
  padding: 24px 16px 40px;
  display: flex;
  flex-direction: column;
  gap: 20px;
}
.head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
}
.head h2 {
  margin: 0;
  font-size: 20px;
}
.progress {
  font-size: 13px;
  color: var(--muted);
}

.state {
  padding: 48px 0;
  text-align: center;
  color: var(--muted);
}
.done p {
  margin: 0 0 8px;
  font-size: 20px;
  color: var(--text);
}
.done .sub {
  font-size: 14px;
  color: var(--muted);
}
.reload-btn {
  margin-top: 16px;
  border: 1px solid var(--border);
  background: var(--card-bg);
  color: var(--accent);
  padding: 8px 18px;
  border-radius: 8px;
  cursor: pointer;
}

/* 3D 翻转：外层提供透视，内层负责 rotateY，前后两面子叠在同一 grid 格 */
.flip {
  perspective: 1200px;
  cursor: pointer;
}
.flip-inner {
  position: relative;
  display: grid;
  transform-style: preserve-3d;
  transition: transform 0.5s ease;
}
.flip.flipped .flip-inner {
  transform: rotateY(180deg);
}
.face {
  grid-area: 1 / 1;
  backface-visibility: hidden;
  -webkit-backface-visibility: hidden;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  padding: 28px 24px;
  min-height: 300px;
  border: 1px solid var(--border);
  border-radius: 16px;
  background: var(--card-bg);
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.05);
}
.face.back {
  transform: rotateY(180deg);
}
.face .word {
  font-size: 34px;
  font-weight: 700;
}
.face .hint {
  font-size: 13px;
  color: var(--muted);
}
.senses {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.sense {
  display: flex;
  align-items: baseline;
  gap: 8px;
  flex-wrap: wrap;
  justify-content: center;
}
.sense .pos {
  font-size: 12px;
  color: var(--accent);
  background: var(--accent-soft);
  padding: 2px 6px;
  border-radius: 5px;
  white-space: nowrap;
}
.sense .translation {
  font-size: 18px;
  color: var(--text);
}
.sense .translation.pending {
  color: var(--muted);
  font-style: italic;
}
.sense .example {
  display: flex;
  flex-direction: column;
  gap: 1px;
  flex-basis: 100%;
}
.sense .example-en {
  font-size: 13px;
  color: var(--muted);
  font-style: italic;
}
.sense .example-trans {
  font-size: 13px;
  color: var(--muted);
}
.face .phonetic {
  font-size: 16px;
  color: var(--muted);
}
.face .speak-btn {
  border: none;
  background: none;
  cursor: pointer;
  padding: 0 2px;
  font-size: 16px;
  line-height: 1;
  opacity: 0.7;
  transition: opacity 0.15s;
}
.face .speak-btn:hover {
  opacity: 1;
}
.face .front-example {
  font-size: 13px;
  color: var(--muted);
  font-style: italic;
  text-align: center;
  line-height: 1.5;
}
.enrich {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 10px;
  justify-content: center;
  font-size: 12px;
  color: var(--muted);
}
.enrich-label {
  font-weight: 600;
  color: var(--text);
}
.face .count {
  font-size: 12px;
  color: var(--muted);
}

.error {
  color: var(--danger);
  font-size: 13px;
  text-align: center;
  margin: 0;
}

.actions {
  display: flex;
  gap: 12px;
}
.rate-btn {
  flex: 1;
}

@media (max-width: 480px) {
  .face { min-height: 260px; }
  .face .word { font-size: 28px; }
}
</style>
