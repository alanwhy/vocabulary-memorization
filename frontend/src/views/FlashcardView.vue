<script setup>
import { computed, onMounted, onBeforeUnmount, ref, watch } from 'vue'
import { apiPut } from '@/api/client'
import { useVocabularyIndex } from '@/composables/useVocabularyIndex'
import { useWordLookup } from '@/composables/useWordLookup'
import { useReviewColors } from '@/composables/useReviewColors'
import { useFlashcardQueue } from '@/composables/useFlashcardQueue'
import { speakWord } from '@/composables/usePronunciation'
import { tokenizeExample, splitWordRef } from '@/utils/highlight'
import { countBadgeClass, reviewColorStyle } from '@/utils/reviewLevel'
import WordCorrectionButton from '@/components/word/WordCorrectionButton.vue'
import { splitGlosses } from '@/utils/gloss'

const vocab = useVocabularyIndex()
const lookup = useWordLookup()
const { reviewColors, ensureReviewColors } = useReviewColors()
const {
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
} = useFlashcardQueue()
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
  const mk = (label, list) => ({ label, refs: (list || []).map((r) => splitWordRef(r)) })
  return [
    mk('近义词', firstSense.value.synonyms),
    mk('反义词', firstSense.value.antonyms),
    mk('派生词', firstSense.value.derivatives),
    mk('形近词', firstSense.value.lookalikes),
  ].filter((g) => g.refs.length)
})
const hasEnrichment = computed(() => !!(rootAffix.value || enrichGroups.value.length))
// 背面每个词性拆分后的义项数组，与 current.senses 一一对应（下标相同）
const senseGlosses = computed(() =>
  (current.value?.senses || []).map((s) => splitGlosses(s.translation)),
)
// 当前词的「重要释义」义项集合，命中即加粗
const importantSet = computed(() => new Set(current.value?.important_glosses || []))

// 点击一个中文义项：切换它是否为「重要释义」，乐观更新 + 写回后端，失败回滚
async function toggleGloss(gloss) {
  const wd = current.value
  if (!wd) return
  const set = new Set(wd.important_glosses || [])
  if (set.has(gloss)) set.delete(gloss)
  else set.add(gloss)
  const next = Array.from(set)
  const prev = wd.important_glosses || []
  wd.important_glosses = next
  try {
    await apiPut(`/api/words/${wd.id}/important`, { glosses: next })
  } catch (e) {
    wd.important_glosses = prev
    error.value = e.message || '标记失败'
  }
}

function exampleTokens(s) {
  // 闪卡仅突出正在复习的词，避免例句中的其它已收录词提前泄露提示；它们仍可点击查词。
  const currentKey = current.value?.word_key
  return tokenizeExample(
    s.example || '',
    (key) => (key === currentKey ? vocab.lookup(key) : 0),
    currentKey,
  )
}

// 点击例句里的某个 token：单词打开查词 tooltip 并拦截冒泡，非单词放行（可翻面）
function onTokenClick(e, t) {
  if (!t.isWord) return
  e.stopPropagation()
  lookup.open(e, t.text)
}

// 点击近反义/形近词里的英文词：打开查词 tooltip 并拦截冒泡，避免触发翻面
function openLookup(e, word) {
  e.stopPropagation()
  lookup.open(e, word)
}
const progress = computed(() => `${Math.min(index.value + 1, cards.value.length)} / ${cards.value.length}`)

// “模糊”表示看例句后认出，会比“不认识”更晚复习；“记住”仍会直接归档。
const RATINGS = [
  { key: 'again', label: '不认识', resultLabel: '不认识', emoji: '❌', type: 'danger' },
  { key: 'fuzzy', label: '模糊', resultLabel: '模糊', emoji: '🤔', type: 'warning' },
  { key: 'good', label: '记住', resultLabel: '认识', emoji: '✅', type: 'success' },
]

// 结果只回顾待加强的词，释义和读音复用本组卡片数据，不带例句或学习记录。
const resultGroups = computed(() =>
  RATINGS.filter((rating) => rating.key !== 'good').map((rating) => ({
    ...rating,
    words: groupResults.value
      .filter((result) => result.rating === rating.key)
      .map(({ word }) => ({
        id: word.id,
        wordKey: word.word_key,
        displayWord: word.display_word,
        phonetic: (word.senses || []).find((sense) => sense.phonetic)?.phonetic || '',
        translation: (word.senses || []).map((sense) => sense.translation).filter(Boolean).join('；'),
      })),
  })),
)

// 每次展示新卡片（加载/切换到下一张）时自动朗读一次，翻面不重复触发
watch(() => current.value?.id, () => {
  if (current.value) speakWord(current.value.word_key)
})
const correcting = ref(false)
let contentTimer
onBeforeUnmount(() => clearInterval(contentTimer))

onMounted(() => {
  ensureQueue()
  refreshContent()
  contentTimer = setInterval(refreshContent, 2000)
  vocab.ensure()
  ensureReviewColors()
})
</script>

<template>
  <main class="flashcards">
    <header class="head">
      <h2>闪卡复习</h2>
      <span v-if="cards.length" class="progress">{{ progress }}</span>
    </header>

    <p v-if="error" class="error" role="alert">{{ error }}</p>

    <div v-if="loading" class="state">加载中...</div>

    <div v-else-if="allDone" class="state done">
      <p>今日复习完成 🎉</p>
      <p class="sub">本次共复习 {{ doneCount }} 张卡片</p>
    </div>

    <section v-else-if="finished" class="group-result" aria-label="本组复习结果">
      <div class="state done result-heading">
        <p>本组完成 🎉</p>
        <p class="sub">本组背完 {{ groupResults.length }} 个单词，累计 {{ doneCount }} 个</p>
      </div>

      <div class="rating-summary" role="list" aria-label="本组认识情况">
        <div v-for="rating in RATINGS" :key="rating.key" class="rating-stat" :class="rating.key" role="listitem">
          <strong>{{ groupCounts[rating.key] }}<small> 个</small></strong>
          <span>{{ rating.resultLabel }}</span>
        </div>
      </div>

      <section v-for="group in resultGroups" :key="group.key" class="result-group" :aria-labelledby="`result-${group.key}`">
        <h3 :id="`result-${group.key}`">{{ group.resultLabel }} <span>{{ group.words.length }} 个</span></h3>
        <ul v-if="group.words.length" class="result-words">
          <li v-for="word in group.words" :key="word.id" class="result-word">
            <div class="result-word-head">
              <strong>{{ word.displayWord }}</strong>
              <span class="result-pronunciation">
                <span v-if="word.phonetic" class="phonetic">{{ word.phonetic }}</span>
                <button type="button" class="speak-btn" :aria-label="`朗读 ${word.displayWord}`" @click="speakWord(word.wordKey)">🔊</button>
              </span>
            </div>
            <p>{{ word.translation || '暂无释义' }}</p>
          </li>
        </ul>
        <p v-else class="result-empty">本组没有{{ group.resultLabel }}的单词</p>
      </section>

      <button class="reload-btn" @click="loadQueue">再来一组</button>
    </section>

    <div v-else-if="!current" class="state">
      <button class="reload-btn" @click="loadQueue">重新加载</button>
    </div>

    <template v-else>
      <WordCorrectionButton :word="current" :disabled="submitting" @busy="correcting = $event" @corrected="updateContent" />
      <p v-if="current.translating" class="state" role="status">正在补全单词内容...</p>
      <div class="flip" :class="{ flipped }" @click="!correcting && flip()">
        <div class="flip-inner">
          <div class="face front">
            <span class="front-count" :class="countBadgeClass(current.review_count)" :style="reviewColorStyle(current.review_count, reviewColors)">×{{ current.review_count }}</span>
            <span class="word">{{ current.display_word }}</span>
            <span class="phonetic-row">
              <span class="phonetic" v-if="phonetic">{{ phonetic }}</span>
              <button
                type="button"
                class="speak-btn"
                :aria-label="`朗读 ${current.display_word}`"
                @click.stop="speakWord(current.word_key)"
              >
                🔊
              </button>
            </span>
            <span class="front-example" v-if="frontExample">
              <span
                v-for="(t, i) in exampleTokens(frontExample)"
                :key="i"
                :class="[t.isWord ? 'lookup-word' : '', t.count ? 'hl-word' : '']"
                :style="t.count ? reviewColorStyle(t.count, reviewColors) : undefined"
                @click="onTokenClick($event, t)"
              >{{ t.text }}</span>
            </span>
            <span class="hint">点击翻面看释义</span>
          </div>
          <div class="face back">
            <span class="word">{{ current.display_word }}</span>
            <span class="phonetic-row">
              <span class="phonetic" v-if="phonetic">{{ phonetic }}</span>
              <button
                type="button"
                class="speak-btn"
                :aria-label="`朗读 ${current.display_word}`"
                @click.stop="speakWord(current.word_key)"
              >
                🔊
              </button>
            </span>
            <div class="senses" v-if="current.senses && current.senses.length">
              <div class="sense" v-for="(s, i) in current.senses" :key="i">
                <span class="pos" v-if="s.pos">{{ s.pos }}</span>
                <span class="translation">
                  <template v-for="(g, gi) in senseGlosses[i]" :key="gi">
                    <span v-if="gi > 0">；</span><span
                      class="gloss"
                      :class="{ 'gloss-important': importantSet.has(g) }"
                      @click.stop="toggleGloss(g)"
                    >{{ g }}</span>
                  </template>
                </span>
                <span class="example" v-if="s.example || s.example_translation">
                  <span class="example-en" v-if="s.example">
                    <span
                      v-for="(t, i) in exampleTokens(s)"
                      :key="i"
                      :class="[t.isWord ? 'lookup-word' : '', t.count ? 'hl-word' : '']"
                      :style="t.count ? reviewColorStyle(t.count, reviewColors) : undefined"
                      @click="onTokenClick($event, t)"
                    >{{ t.text }}</span>
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
                <span class="enrich-label">{{ g.label }}：</span><span v-for="(r, i) in g.refs" :key="i">{{ i > 0 ? '、' : '' }}<span class="lookup-word" :class="{ 'known-word': vocab.lookup(r.word.toLowerCase()) }" @click="openLookup($event, r.word)">{{ r.word }}</span>{{ r.rest }}</span>
              </span>
            </div>
            <span class="count">已背 ×{{ current.review_count }}</span>
          </div>
        </div>
      </div>

      <div class="actions">
        <el-button
          v-for="r in RATINGS"
          :key="r.key"
          class="rate-btn"
          :type="r.type"
          size="large"
          :disabled="submitting || correcting || current.translating"
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

.group-result {
  display: flex;
  flex-direction: column;
  gap: 20px;
}
.result-heading {
  padding: 8px 0 0;
}
.rating-summary {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
}
.rating-stat {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  padding: 16px 8px;
  border: 1px solid var(--border);
  border-radius: 12px;
  background: var(--card-bg);
}
.rating-stat strong {
  font-size: 26px;
}
.rating-stat small,
.rating-stat > span {
  font-size: 13px;
  font-weight: 400;
  color: var(--muted);
}
.rating-stat.again strong {
  color: var(--danger);
}
.rating-stat.fuzzy strong {
  color: var(--el-color-warning);
}
.rating-stat.good strong {
  color: var(--el-color-success);
}
.result-group h3 {
  display: flex;
  align-items: baseline;
  gap: 8px;
  margin: 0 0 10px;
  font-size: 16px;
}
.result-group h3 span {
  color: var(--muted);
  font-size: 13px;
  font-weight: 400;
}
.result-words {
  margin: 0;
  padding: 0;
  list-style: none;
  border: 1px solid var(--border);
  border-radius: 12px;
  background: var(--card-bg);
}
.result-word {
  padding: 14px 16px;
  overflow-wrap: anywhere;
}
.result-word + .result-word {
  border-top: 1px solid var(--border);
}
.result-word-head,
.result-pronunciation {
  display: flex;
  align-items: center;
  gap: 8px;
}
.result-word-head {
  flex-wrap: wrap;
}
.result-word-head > strong {
  font-size: 18px;
}
.result-pronunciation .phonetic {
  color: var(--muted);
  font-size: 13px;
}
.result-pronunciation .speak-btn {
  flex-shrink: 0;
  border: none;
  background: none;
  padding: 4px;
  font-size: 16px;
  cursor: pointer;
}
.result-word p {
  margin: 6px 0 0;
  font-size: 14px;
  line-height: 1.6;
}
.result-empty {
  margin: 0;
  color: var(--muted);
  font-size: 14px;
}
.group-result .reload-btn {
  align-self: center;
  margin-top: 0;
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
  position: relative;
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
.sense .gloss {
  cursor: pointer;
}
.sense .gloss-important {
  font-weight: 700;
  font-style: italic;
  color: var(--danger);
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
.face .phonetic-row {
  display: flex;
  align-items: center;
  gap: 4px;
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
  flex-direction: column;
  align-items: flex-start;
  gap: 4px;
  font-size: 12px;
  color: var(--muted);
}
.enrich-label {
  font-weight: 600;
  color: var(--text);
}
.face .count {
  position: absolute;
  top: 14px;
  right: 14px;
  font-size: 12px;
  color: var(--muted);
}
.face .front-count {
  position: absolute;
  top: 14px;
  right: 14px;
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
