<script setup>
import { computed, onMounted } from 'vue'
import { formatTime } from '@/utils/format'
import { countBadgeClass } from '@/utils/reviewLevel'
import { useVocabularyIndex } from '@/composables/useVocabularyIndex'
import { tokenizeExample, splitWordRef } from '@/utils/highlight'

const props = defineProps({
  word: { type: Object, required: true },
  mode: { type: String, default: 'active' }, // 'active' | 'archived'
})
defineEmits(['archive', 'unarchive', 'delete'])

const vocab = useVocabularyIndex()
onMounted(vocab.ensure)

// 词级强化信息（音标/词根词缀/近反义/形近词）平铺在每条 Sense 上重复出现，统一从第一条取
const first = computed(() => (props.word.senses && props.word.senses[0]) || {})
const phonetic = computed(() => first.value.phonetic || '')
const rootAffix = computed(() => {
  const parts = [first.value.root, first.value.affix].filter(Boolean)
  return parts.join(' + ') // 只返回词根词缀内容，「词根词缀：」标题由模板统一渲染
})

// 近义词/反义词/形近词统一拆成「英文词 + 中文释义」，英文词按词库命中情况高亮
const enrichGroups = computed(() => {
  const mk = (label, list) => ({ label, refs: (list || []).map((r) => splitWordRef(r, vocab.lookup)) })
  return [
    mk('近义词', first.value.synonyms),
    mk('反义词', first.value.antonyms),
    mk('形近词', first.value.lookalikes),
  ].filter((g) => g.refs.length)
})
const hasEnrichment = computed(() => !!(rootAffix.value || enrichGroups.value.length))

// 高亮 class：命中词库的英文词按出现次数档位着色（1~6）
function hlClass(level) {
  return `hl-word hl-word--l${level}`
}

function exampleTokens(s) {
  return tokenizeExample(s.example || '', vocab.lookup)
}
</script>

<template>
  <li class="word-card">
    <div class="word-top">
      <div class="word-line">
        <span class="word">{{ word.display_word }}</span>
        <span class="phonetic" v-if="phonetic">{{ phonetic }}</span>
      </div>
      <div class="word-meta">
        <span :class="countBadgeClass(word.review_count)">×{{ word.review_count }}</span>
        <span class="time">{{ formatTime(word.last_reviewed_at) }}</span>
      </div>
    </div>
    <div class="word-body">
      <div class="senses" v-if="word.senses && word.senses.length">
        <div class="sense" v-for="(s, idx) in word.senses" :key="idx">
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
      <div class="senses" v-else-if="word.translating">
        <div class="sense"><span class="translation pending">查词中...</span></div>
      </div>
      <div class="senses" v-else>
        <div class="sense"><span class="translation">暂无释义</span></div>
      </div>
      <div class="enrich" v-if="hasEnrichment">
        <span v-if="rootAffix">
          <span class="enrich-label">词根词缀：</span>{{ rootAffix }}
        </span>
        <span v-for="g in enrichGroups" :key="g.label">
          <span class="enrich-label">{{ g.label }}：</span><span v-for="(r, i) in g.refs" :key="i">{{ i > 0 ? '、' : '' }}<span :class="r.level ? hlClass(r.level) : ''">{{ r.word }}</span>{{ r.rest }}</span>
        </span>
      </div>
    </div>
    <div class="word-actions">
      <button v-if="mode === 'active'" class="action-btn primary" @click="$emit('archive', word)">归档</button>
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
  flex-wrap: wrap;
  gap: 4px 12px;
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
