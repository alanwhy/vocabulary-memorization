<script setup>
import { onMounted, ref } from 'vue'
import { apiGet } from '@/api/client'
import { useWordActions } from '@/composables/useWordActions'
import WordList from '@/components/word/WordList.vue'

const words = ref([])
const { unarchiveWord, deleteWord } = useWordActions(words)

async function loadWords() {
  words.value = await apiGet('/api/words?archived=1')
}

onMounted(loadWords)
</script>

<template>
  <div>
    <h1>归档</h1>
    <WordList
      :words="words"
      mode="archived"
      empty-text="还没有归档的单词"
      @unarchive="unarchiveWord"
      @delete="deleteWord"
    />
  </div>
</template>
