<script setup>
import { computed, onBeforeUnmount, watch } from 'vue'
import { useWordLookup } from '@/composables/useWordLookup'
import { speakWord } from '@/composables/usePronunciation'

const { state, close } = useWordLookup()

// 音标/词级信息从第一条词性取（平铺模型下每条重复）
const firstSense = computed(() => (state.data?.senses && state.data.senses[0]) || {})
const phonetic = computed(() => firstSense.value.phonetic || '')

// 定位：以点击点为中心横向夹在视口内；靠近底部时向上弹出
const style = computed(() => {
  const width = 320
  const left = Math.min(Math.max(state.x - width / 2, 8), window.innerWidth - width - 8)
  const flipUp = state.y > window.innerHeight - 320
  const top = flipUp ? state.y - 12 : state.y + 12
  return {
    left: `${left}px`,
    top: `${top}px`,
    transform: flipUp ? 'translateY(-100%)' : 'none',
  }
})

// 点击 tooltip 之外关闭；tooltip 与单词本身都 @click.stop，所以只有真正点到空白处才会触发
function onDocClick() {
  close()
}

watch(
  () => state.open,
  (open) => {
    if (open) document.addEventListener('click', onDocClick)
    else document.removeEventListener('click', onDocClick)
  },
)

onBeforeUnmount(() => document.removeEventListener('click', onDocClick))
</script>

<template>
  <Teleport to="body">
    <div v-if="state.open" class="lookup-tooltip" :style="style" @click.stop>
      <header class="tt-head">
        <span class="tt-word">{{ state.word }}</span>
        <span class="tt-phonetic" v-if="phonetic">{{ phonetic }}</span>
        <button
          type="button"
          class="tt-speak"
          :aria-label="`朗读 ${state.word}`"
          @click.stop="speakWord(state.wordKey)"
        >
          🔊
        </button>
      </header>

      <div class="tt-body">
        <p v-if="state.loading" class="tt-state">查询中...</p>
        <p v-else-if="state.error" class="tt-state tt-error">{{ state.error }}</p>
        <template v-else-if="state.data">
          <p v-if="state.data.translating" class="tt-state">查词中...</p>
          <template v-else-if="state.data.senses && state.data.senses.length">
            <div class="tt-sense" v-for="(s, i) in state.data.senses" :key="i">
              <span class="tt-line">
                <span class="tt-pos" v-if="s.pos">{{ s.pos }}</span>
                <span class="tt-translation">{{ s.translation }}</span>
              </span>
              <span class="tt-example" v-if="s.example || s.example_translation">
                <span class="tt-example-en" v-if="s.example">{{ s.example }}</span>
                <span class="tt-example-trans" v-if="s.example_translation">{{ s.example_translation }}</span>
              </span>
            </div>
          </template>
          <p v-else class="tt-state">暂无释义</p>
        </template>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.lookup-tooltip {
  position: fixed;
  z-index: 3000;
  min-width: 240px;
  max-width: 320px;
  max-height: 60vh;
  overflow-y: auto;
  background: var(--card-bg);
  border: 1px solid var(--border);
  border-radius: 10px;
  box-shadow: 0 6px 24px rgba(0, 0, 0, 0.12);
  padding: 12px 14px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.tt-head {
  display: flex;
  align-items: baseline;
  gap: 8px;
  flex-wrap: wrap;
}
.tt-word {
  font-size: 17px;
  font-weight: 700;
  color: var(--text);
}
.tt-phonetic {
  font-size: 13px;
  color: var(--muted);
}
.tt-speak {
  border: none;
  background: none;
  cursor: pointer;
  padding: 0 2px;
  font-size: 15px;
  line-height: 1;
  opacity: 0.75;
  transition: opacity 0.15s;
}
.tt-speak:hover {
  opacity: 1;
}
.tt-body {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.tt-state {
  margin: 0;
  font-size: 13px;
  color: var(--muted);
  font-style: italic;
}
.tt-state.tt-error {
  color: var(--danger);
}
.tt-sense {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.tt-line {
  display: flex;
  align-items: baseline;
  gap: 6px;
  flex-wrap: wrap;
}
.tt-pos {
  font-size: 12px;
  color: var(--accent);
  background: var(--accent-soft);
  padding: 1px 5px;
  border-radius: 4px;
  white-space: nowrap;
}
.tt-translation {
  font-size: 14px;
  color: var(--text);
}
.tt-example {
  display: flex;
  flex-direction: column;
  gap: 1px;
}
.tt-example-en {
  font-size: 12px;
  color: var(--muted);
  font-style: italic;
}
.tt-example-trans {
  font-size: 12px;
  color: var(--muted);
}
</style>
