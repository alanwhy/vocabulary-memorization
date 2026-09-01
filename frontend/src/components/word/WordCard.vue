<script setup>
import { computed, onMounted } from 'vue'
import { formatTime } from '@/utils/format'
import { countBadgeClass } from '@/utils/reviewLevel'
import { useVocabularyIndex } from '@/composables/useVocabularyIndex'
import { useWordLookup } from '@/composables/useWordLookup'
import { speakWord } from '@/composables/usePronunciation'
import { tokenizeExample, splitWordRef } from '@/utils/highlight'
import { splitGlosses } from '@/utils/gloss'

const props = defineProps({
  word: { type: Object, required: true },
  mode: { type: String, default: 'active' }, // 'active' | 'archived'
})
const emit = defineEmits(['archive', 'unarchive', 'delete', 'retry', 'set-important'])

const vocab = useVocabularyIndex()
const lookup = useWordLookup()
onMounted(vocab.ensure)

// 错误态：释义里含占位性质的 sense（新版拼写错误/查询失败用 pos === 'error'，旧版查询失败用 pos === '系统提示'）。
// 前端据此显示重试而非归档，且重试按钮对所有用户（含管理员）都生效——判断只依赖单词状态，不依赖身份。
const errorSenses = computed(() =>
  (props.word.senses || []).filter((s) => s.pos === 'error' || s.pos === '系统提示'),
)
const hasError = computed(() => errorSenses.value.length > 0)
// 正常释义：排除占位 sense 后的有效词性释义
const validSenses = computed(() =>
  (props.word.senses || []).filter((s) => s.pos !== 'error' && s.pos !== '系统提示'),
)

// 词级强化信息（音标/词根词缀/近反义/形近词）平铺在每条 Sense 上重复出现，统一从第一条取
const first = computed(() => validSenses.value[0] || {})
const phonetic = computed(() => first.value.phonetic || '')
const rootAffix = computed(() => {
  const parts = [first.value.root, first.value.affix].filter(Boolean)
  return parts.join(' + ') // 只返回词根词缀内容，「词根词缀：」标题由模板统一渲染
})

// 近义词/反义词/形近词统一拆成「英文词 + 中文释义」，英文词按词库命中情况高亮
const enrichGroups = computed(() => {
  const mk = (label, list) => ({ label, refs: (list || []).map((r) => splitWordRef(r)) })
  return [
    mk('近义词', first.value.synonyms),
    mk('反义词', first.value.antonyms),
    mk('形近词', first.value.lookalikes),
  ].filter((g) => g.refs.length)
})
const hasEnrichment = computed(() => !!(rootAffix.value || enrichGroups.value.length))

// 当前词的「重要释义」义项集合，命中即加粗
const importantSet = computed(() => new Set(props.word.important_glosses || []))
// 一个词性的中文释义按「；」拆成义项
function glossItems(s) {
  return splitGlosses(s.translation)
}
// 点击一个中文义项：计算新的全量义项列表并 emit 给父级处理（受控组件，不直接改 prop）
function toggleGloss(gloss) {
  const set = new Set(props.word.important_glosses || [])
  if (set.has(gloss)) set.delete(gloss)
  else set.add(gloss)
  emit('set-important', { id: props.word.id, glosses: Array.from(set) })
}

// 高亮 class：命中词库的英文词按出现次数档位着色（1~6）
function hlClass(level) {
  return `hl-word hl-word--l${level}`
}

function exampleTokens(s) {
  return tokenizeExample(s.example || '', vocab.lookup, props.word.word_key)
}

// 点击例句里的某个 token：单词打开查词 tooltip，非单词忽略
function onTokenClick(e, t) {
  if (!t.isWord) return
  e.stopPropagation()
  lookup.open(e, t.text)
}

// 点击近反义/形近词里的英文词：打开查词 tooltip
function openLookup(e, word) {
  e.stopPropagation()
  lookup.open(e, word)
}
</script>

<template>
  <li class="word-card">
    <div class="word-top">
      <div class="word-line">
        <span class="word">{{ word.display_word }}</span>
        <span class="translating-status" v-if="word.translating">查询中...</span>
        <span class="phonetic" v-if="phonetic">{{ phonetic }}</span>
        <button
          type="button"
          class="speak-btn"
          :aria-label="`朗读 ${word.display_word}`"
          @click="speakWord(word.word_key)"
        >
          🔊
        </button>
      </div>
      <div class="word-meta">
        <span :class="countBadgeClass(word.review_count)">×{{ word.review_count }}</span>
        <span class="time">{{ formatTime(word.last_reviewed_at) }}</span>
      </div>
    </div>
    <div class="word-body">
      <div class="senses" v-if="word.translating">
        <div class="sense"><span class="translation pending">查词中...</span></div>
      </div>
      <div class="senses" v-else-if="hasError">
        <div class="sense" v-for="(s, idx) in errorSenses" :key="idx">
          <span class="error-label">error：</span>
          <span class="translation error">{{ s.translation }}</span>
        </div>
      </div>
      <div class="senses" v-else-if="validSenses.length">
        <div class="sense" v-for="(s, idx) in validSenses" :key="idx">
          <span class="pos" v-if="s.pos">{{ s.pos }}</span>
          <span class="translation">
            <span
              v-for="(g, gi) in glossItems(s)"
              :key="gi"
              class="gloss"
              :class="{ 'gloss-important': importantSet.has(g) }"
              @click.stop="toggleGloss(g)"
            >{{ gi > 0 ? '；' : '' }}{{ g }}</span>
          </span>
          <span class="example" v-if="s.example || s.example_translation">
            <span class="example-en" v-if="s.example">
              <span
                v-for="(t, i) in exampleTokens(s)"
                :key="i"
                :class="[t.isWord ? 'lookup-word' : '', t.level ? hlClass(t.level) : '']"
                @click="onTokenClick($event, t)"
              >{{ t.text }}</span>
            </span>
            <span class="example-trans" v-if="s.example_translation">{{ s.example_translation }}</span>
          </span>
        </div>
      </div>
      <div class="senses" v-else>
        <div class="sense"><span class="translation">暂无释义</span></div>
      </div>
      <div class="enrich" v-if="!word.translating && hasEnrichment">
        <span v-if="rootAffix">
          <span class="enrich-label">词根词缀：</span>{{ rootAffix }}
        </span>
        <span v-for="g in enrichGroups" :key="g.label">
          <span class="enrich-label">{{ g.label }}：</span><span v-for="(r, i) in g.refs" :key="i">{{ i > 0 ? '、' : '' }}<span class="lookup-word" @click="openLookup($event, r.word)">{{ r.word }}</span>{{ r.rest }}</span>
        </span>
      </div>
    </div>
    <div class="word-actions">
      <template v-if="mode === 'active'">
        <button
          v-if="word.archived"
          class="action-btn archived-btn"
          disabled
          title="该词已归档，刷新后将从当前列表消失"
        >
          已归档
        </button>
        <button
          v-else-if="!word.translating && !hasError && validSenses.length"
          class="action-btn primary"
          @click="$emit('archive', word)"
        >
          归档
        </button>
        <button v-else-if="!word.translating" class="action-btn primary" @click="$emit('retry', word)">
          重试
        </button>
      </template>
      <button v-else class="action-btn primary" @click="$emit('unarchive', word)">取消归档</button>
      <button class="action-btn danger" @click="$emit('delete', word)">删除</button>
    </div>
  </li>
</template>

<style scoped>
.word-card {
  background: var(--card-bg);
  border: 1px solid var(--border);
  border-radius: 10px;
  padding: 14px 16px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
/* 顶行：单词 + 音标靠左，次数 + 日期靠最右 */
.word-top {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 12px;
  min-width: 0;
}
.word-line {
  display: flex;
  align-items: baseline;
  gap: 8px;
  min-width: 0;
}
.word {
  font-size: 17px;
  font-weight: 600;
}
.phonetic {
  font-size: 13px;
  color: var(--muted);
}
.speak-btn {
  border: none;
  background: none;
  cursor: pointer;
  padding: 0 2px;
  font-size: 14px;
  line-height: 1;
  opacity: 0.7;
  transition: opacity 0.15s;
}
.speak-btn:hover {
  opacity: 1;
}
.translating-status {
  font-size: 12px;
  color: var(--muted);
  font-style: italic;
  white-space: nowrap;
}
.word-body {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}
.senses {
  display: flex;
  flex-direction: column;
  gap: 3px;
}
.sense {
  display: flex;
  align-items: baseline;
  gap: 8px;
  flex-wrap: wrap;
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
  font-size: 14px;
  color: var(--text);
  word-break: break-word;
}
.sense .translation.pending {
  color: var(--muted);
  font-style: italic;
}
.sense .translation.error {
  color: var(--danger);
}
.sense .gloss {
  cursor: pointer;
}
.sense .gloss-important {
  font-weight: 700;
  font-style: italic;
  color: var(--danger);
}
.error-label {
  font-weight: 600;
  color: var(--danger);
}
.sense .example {
  display: flex;
  flex-direction: column;
  gap: 1px;
  flex-basis: 100%;
}
.sense .example-en {
  font-size: 12px;
  color: var(--muted);
  font-style: italic;
}
.sense .example-trans {
  font-size: 12px;
  color: var(--muted);
}
.enrich {
  display: flex;
  flex-direction: column;
  gap: 4px;
  margin-top: 2px;
  font-size: 12px;
  color: var(--muted);
}
.enrich-label {
  font-weight: 600;
  color: var(--text);
}
.word-meta {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-shrink: 0;
}
.word-meta .time {
  font-size: 12px;
  color: var(--muted);
  white-space: nowrap;
}
/* 底部操作按钮：Element 主题色（归档/取消归档）+ 删除色（删除），一眼区分可点击操作 */
.word-actions {
  display: flex;
  gap: 8px;
  margin-top: 2px;
}
.action-btn {
  font-size: 12px;
  cursor: pointer;
  padding: 3px 12px;
  border-radius: 6px;
  border: 1px solid;
  background: transparent;
  transition: background 0.15s, color 0.15s;
}
.action-btn.primary {
  color: var(--accent);
  border-color: var(--accent);
  background: var(--accent-soft);
}
.action-btn.primary:hover {
  background: var(--accent);
  color: #fff;
}
.action-btn.archived-btn {
  color: var(--muted);
  border-color: var(--border);
  cursor: not-allowed;
}
.action-btn.danger {
  color: var(--danger);
  border-color: var(--danger);
}
.action-btn.danger:hover {
  background: var(--danger);
  color: #fff;
}

@media (max-width: 480px) {
  .word-top {
    flex-wrap: wrap;
  }
}
</style>
