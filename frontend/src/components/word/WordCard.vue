<script setup>
defineProps({
  word: { type: Object, required: true },
  mode: { type: String, default: 'active' }, // 'active' | 'archived'
})
defineEmits(['archive', 'unarchive', 'delete'])

function formatTime(iso) {
  const d = new Date(iso)
  const pad = (n) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}
</script>

<template>
  <li class="word-card">
    <div class="word-main">
      <span class="word">{{ word.display_word }}</span>
      <div class="senses" v-if="word.senses && word.senses.length">
        <div class="sense" v-for="(s, idx) in word.senses" :key="idx">
          <span class="pos" v-if="s.pos">{{ s.pos }}</span>
          <span class="translation">{{ s.translation }}</span>
        </div>
      </div>
      <div class="senses" v-else-if="word.translating">
        <div class="sense"><span class="translation pending">查词中...</span></div>
      </div>
      <div class="senses" v-else>
        <div class="sense"><span class="translation">暂无释义</span></div>
      </div>
    </div>
    <div class="word-meta">
      <span class="count">×{{ word.review_count }}</span>
      <span class="time">{{ formatTime(word.last_reviewed_at) }}</span>
      <button v-if="mode === 'active'" class="archive-btn" @click="$emit('archive', word)">归档</button>
      <button v-else class="unarchive-btn" @click="$emit('unarchive', word)">取消归档</button>
      <button class="delete-btn" @click="$emit('delete', word)">删除</button>
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
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}
.word-main {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}
.word-main .word {
  font-size: 17px;
  font-weight: 600;
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
.word-meta {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-shrink: 0;
}
.word-meta .count {
  font-size: 13px;
  font-weight: 600;
  color: var(--accent);
  background: var(--accent-soft);
  padding: 3px 8px;
  border-radius: 999px;
}
.word-meta .time {
  font-size: 12px;
  color: var(--muted);
  white-space: nowrap;
}
.delete-btn, .archive-btn, .unarchive-btn {
  border: none;
  background: transparent;
  color: var(--muted);
  font-size: 12px;
  cursor: pointer;
  padding: 4px 6px;
}
.delete-btn:hover { color: var(--danger); }
.archive-btn:hover, .unarchive-btn:hover { color: var(--accent); }

@media (max-width: 480px) {
  .word-card {
    flex-direction: column;
    align-items: flex-start;
  }
  .word-meta {
    width: 100%;
    justify-content: space-between;
  }
}
</style>
