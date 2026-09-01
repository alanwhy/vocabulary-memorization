<script setup>
import { ref } from 'vue'
import WordCard from './WordCard.vue'
import { useInfiniteScroll } from '@/composables/useInfiniteScroll'

defineProps({
  words: { type: Array, required: true },
  mode: { type: String, default: 'active' }, // 'active' | 'archived'
  emptyText: { type: String, default: '还没有记录' },
  loading: { type: Boolean, default: false },
  hasMore: { type: Boolean, default: false },
})
const emit = defineEmits(['archive', 'unarchive', 'delete', 'retry', 'set-important', 'load-more'])

// 哨兵滚进视口就要下一页；重复触发无害，usePaginatedList 会用 loading / hasMore 挡住
const sentinelRef = ref(null)
useInfiniteScroll(sentinelRef, () => emit('load-more'))
</script>

<template>
  <div>
    <ul class="word-list" v-if="words.length">
      <WordCard
        v-for="w in words"
        :key="w.id"
        :word="w"
        :mode="mode"
        @archive="$emit('archive', w)"
        @unarchive="$emit('unarchive', w)"
        @delete="$emit('delete', w)"
        @retry="$emit('retry', w)"
        @set-important="$emit('set-important', $event)"
      />
    </ul>
    <!-- 首屏加载中先别显示空态，否则会闪一下「还没有记录」 -->
    <div class="empty" v-else-if="!loading">{{ emptyText }}</div>

    <div v-if="hasMore" ref="sentinelRef" class="sentinel"></div>
    <div class="load-state" v-if="loading && words.length">加载中…</div>
    <div class="load-state" v-else-if="!hasMore && words.length">已全部加载</div>
  </div>
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
.sentinel {
  height: 1px;
}
</style>
