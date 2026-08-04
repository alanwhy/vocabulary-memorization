<script setup>
import WordCard from './WordCard.vue'

defineProps({
  words: { type: Array, required: true },
  mode: { type: String, default: 'active' }, // 'active' | 'archived'
  emptyText: { type: String, default: '还没有记录' },
})
defineEmits(['archive', 'unarchive', 'delete'])
</script>

<template>
  <ul class="word-list" v-if="words.length">
    <WordCard
      v-for="w in words"
      :key="w.id"
      :word="w"
      :mode="mode"
      @archive="$emit('archive', w)"
      @unarchive="$emit('unarchive', w)"
      @delete="$emit('delete', w)"
    />
  </ul>
  <div class="empty" v-else>{{ emptyText }}</div>
</template>

<style scoped>
.word-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 10px;
}
</style>
