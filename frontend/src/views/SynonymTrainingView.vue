<script setup>
import { computed, onMounted } from 'vue'
import { useSynonymTraining } from '@/composables/useSynonymTraining'
import '@/assets/training.css'
const flow = useSynonymTraining(), { state } = flow
const snapshot = computed(() => state.snapshot)
const card = computed(() => snapshot.value?.card)
const feedback = computed(() => snapshot.value?.feedback)
/** @param {object} citation 核验后的出处 @returns {string} 标签对应的来源说明。 */
function sourceTitle(citation) { return `${citation.year} 年 · ${citation.title}\n${citation.quote}` }
/** @param {string} id 选项 ID @returns {string} 当前题的显示序号。 */
function letter(id) { const i = card.value?.question?.options?.findIndex(o => o.id === id); return i >= 0 ? String.fromCharCode(65 + i) : '' }
onMounted(flow.initialize)
</script>
<template>
  <main class="training daily-training">
    <h1>同义替换</h1>
    <p v-if="state.error" class="daily-error" role="alert">{{ state.error }} <button class="daily-link" @click="flow.initialize">重新读取</button></p>
    <p v-if="state.loading" class="empty">正在准备今天的练习…</p>
    <section v-else-if="snapshot?.status === 'empty'" class="card daily-empty"><p>{{ snapshot.message || '没有未归档的单词。' }}</p><RouterLink to="/">去词库添加单词</RouterLink><button v-if="snapshot.group_id" class="daily-button" :disabled="state.busy" @click="flow.more">再来一组</button></section>
    <section v-else-if="snapshot?.status === 'complete'" class="card daily-empty"><h2>本组完成</h2><p>这一组 {{ snapshot.total }} 个单词已结束。</p><p v-if="snapshot.message" class="daily-muted">{{ snapshot.message }}</p><button class="daily-button primary" :disabled="state.busy || snapshot.generating" @click="flow.more">再来一组</button></section>
    <section v-else-if="card" class="card daily-card">
      <div class="daily-top"><span>第 {{ snapshot.position }} / {{ snapshot.total }} 个词</span><span>出现 {{ card.count }} 次</span></div>
      <h2 class="daily-word">{{ card.word }}</h2>
      <template v-if="card.question">
        <p class="daily-stem">{{ card.question.stem }} <a v-if="card.question.exam" class="daily-exam" :href="card.question.exam.url" :title="sourceTitle(card.question.exam)" target="_blank" rel="noopener noreferrer">【真题】</a></p>
        <div class="daily-options"><div v-for="(option, index) in card.question.options" :key="option.id" class="daily-choice"><button class="daily-option" :class="{ correct: feedback?.correct_option_id === option.id, wrong: feedback && !feedback.correct && feedback.selected_option_id === option.id }" :disabled="state.busy || !!feedback" @click="flow.answer(option.id)"><strong>{{ String.fromCharCode(65 + index) }}</strong><span>{{ option.text }}</span></button><a v-if="option.exam" class="daily-exam" :href="option.exam.url" :title="sourceTitle(option.exam)" target="_blank" rel="noopener noreferrer" >【真题】</a></div></div>
        <div v-if="feedback" class="daily-feedback" aria-live="polite"><p>{{ feedback.correct ? '答对了' : '正确选项是 ' + letter(feedback.correct_option_id) }} · {{ feedback.meaning }}</p><p class="daily-pair">{{ card.word }} ↔ {{ feedback.replacement }}</p><p>{{ feedback.explanation }}</p><p v-for="(mapping, index) in feedback.mappings" :key="index" class="daily-muted">{{ mapping.original }} ↔ {{ mapping.replacement }}：{{ mapping.note }}</p></div>
        <div class="daily-actions"><button v-if="feedback" class="daily-button primary" :disabled="state.busy" @click="flow.next">下一词</button><button v-else class="daily-button" :disabled="state.busy" @click="flow.answer(null)">不知道</button><button class="daily-button" :disabled="state.busy || snapshot.generating" @click="flow.more">再来一组</button></div>
      </template>
      <template v-else-if="snapshot.status === 'failed'"><p class="daily-error">{{ snapshot.message }}</p><div class="daily-actions"><button class="daily-button primary" :disabled="state.busy" @click="flow.retry">重试本词</button><button class="daily-button" :disabled="state.busy" @click="flow.next">跳过</button></div></template>
      <p v-else class="daily-muted" role="status">正在生成同义替换…</p>
      <p v-if="snapshot.message && snapshot.status !== 'failed'" class="daily-muted">{{ snapshot.message }}</p>
      <p v-if="snapshot.generating && card.question" class="daily-muted">后续单词正在准备（{{ snapshot.generated }}/{{ snapshot.total }}）。</p>
    </section>
    <p class="daily-footer">按未归档单词的出现次数排序 · 当天练习，次日更新</p>
  </main>
</template>
