<script setup>
import { onMounted } from 'vue'
import { apiGet } from '@/api/client'
import { usePaginatedList } from '@/composables/usePaginatedList'
import { useWordActions } from '@/composables/useWordActions'
import WordList from '@/components/word/WordList.vue'

// 归档列表没有排序切换，固定用后端默认的按次数排序
const list = usePaginatedList((page, limit) =>
  apiGet(`/api/words?archived=1&page=${page}&limit=${limit}`),
)
const { items: words, total, loading, hasMore, loaded, errorMsg, loadMore, reset } = list
const { unarchiveWord, deleteWord } = useWordActions(list)

onMounted(reset)
</script>

<template>
  <div>
    <h1>归档</h1>
    <p class="sub" v-if="total">共 {{ total }} 个已归档单词</p>
    <p class="sub error" v-if="errorMsg">{{ errorMsg }}</p>
    <WordList
      :words="words"
      mode="archived"
      empty-text="还没有归档的单词"
      :loading="loading || !loaded"
      :has-more="hasMore"
      @unarchive="unarchiveWord"
      @delete="deleteWord"
      @load-more="loadMore"
    />
  </div>
</template>

<style scoped>
.sub {
  font-size: 13px;
  color: var(--muted);
  margin: 0 4px 12px;
}
.sub.error {
  color: var(--danger);
}
</style>
