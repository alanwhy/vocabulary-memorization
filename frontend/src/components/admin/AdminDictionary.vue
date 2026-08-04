<script setup>
import { computed, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { apiGet, apiDelete } from '@/api/client'

const dictEntries = ref([])
const dictFilter = ref('')
const filteredDictEntries = computed(() => {
  const kw = dictFilter.value.trim().toLowerCase()
  if (!kw) return dictEntries.value
  return dictEntries.value.filter((d) => d.word_key.includes(kw))
})

function formatTime(iso) {
  const d = new Date(iso)
  const pad = (n) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

async function loadDictionary() {
  dictEntries.value = await apiGet('/api/admin/dictionary')
}

async function deleteDictEntry(d) {
  try {
    await ElMessageBox.confirm(`确定从词库删除「${d.display_word}」的缓存记录吗？`, '提示', {
      confirmButtonText: '删除',
      cancelButtonText: '取消',
      type: 'warning',
    })
  } catch {
    return
  }
  try {
    await apiDelete(`/api/admin/dictionary/${encodeURIComponent(d.word_key)}`)
    dictEntries.value = dictEntries.value.filter((x) => x.word_key !== d.word_key)
  } catch {
    ElMessage.error('删除失败，请重试')
  }
}

onMounted(loadDictionary)
</script>

<template>
  <h2>词库管理</h2>
  <div class="card">
    <a class="export-btn" href="/api/admin/dictionary/export">导出 CSV</a>
    <input type="text" v-model="dictFilter" placeholder="按单词过滤" />
    <table style="margin-top: 12px">
      <thead>
        <tr>
          <th>单词</th>
          <th>释义</th>
          <th>最后更新时间</th>
          <th>操作</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="d in filteredDictEntries" :key="d.word_key">
          <td>{{ d.display_word }}</td>
          <td>
            <div class="senses" v-if="d.senses && d.senses.length">
              <div class="sense" v-for="(s, idx) in d.senses" :key="idx">
                <span class="pos" v-if="s.pos">{{ s.pos }}</span>
                <span class="translation">{{ s.translation }}</span>
              </div>
            </div>
            <span v-else>暂无释义</span>
          </td>
          <td>{{ formatTime(d.last_updated_at) }}</td>
          <td><button class="link-btn danger" @click="deleteDictEntry(d)">删除</button></td>
        </tr>
      </tbody>
    </table>
    <p class="msg" v-if="!dictEntries.length">词库暂无数据</p>
  </div>
</template>
