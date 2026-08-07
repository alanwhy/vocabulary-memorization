<script setup>
import { onMounted, onUnmounted, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { apiGet, apiDelete } from '@/api/client'
import { usePaginatedList } from '@/composables/usePaginatedList'
import { useInfiniteScroll } from '@/composables/useInfiniteScroll'
import { formatTime } from '@/utils/format'
import { countBadgeClass } from '@/utils/reviewLevel'

const dictFilter = ref('')

// 过滤交给后端：词库可能有上万条，本地过滤前提是先把整张表拉下来
const list = usePaginatedList((page, limit) =>
  apiGet(
    `/api/admin/dictionary?keyword=${encodeURIComponent(dictFilter.value.trim())}&page=${page}&limit=${limit}`,
  ),
)
const { items: dictEntries, total, loading, hasMore, loaded, errorMsg, reset, loadMore } = list

const sentinelRef = ref(null)
useInfiniteScroll(sentinelRef, loadMore)

// 输入一个字符就打一次接口太浪费，停手 300ms 后再回到第一页重查
let filterTimer = null
watch(dictFilter, () => {
  if (filterTimer) clearTimeout(filterTimer)
  filterTimer = setTimeout(reset, 300)
})
onUnmounted(() => {
  if (filterTimer) clearTimeout(filterTimer)
})

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
    list.removeItem((x) => x.word_key === d.word_key)
  } catch {
    ElMessage.error('删除失败，请重试')
  }
}

onMounted(reset)
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
          <th>出现次数</th>
          <th>最后更新时间</th>
          <th>操作</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="d in dictEntries" :key="d.word_key">
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
          <td><span :class="countBadgeClass(d.occurrence_count)">×{{ d.occurrence_count }}</span></td>
          <td>{{ formatTime(d.last_updated_at) }}</td>
          <td><button class="link-btn danger" @click="deleteDictEntry(d)">删除</button></td>
        </tr>
      </tbody>
    </table>
    <div v-if="hasMore" ref="sentinelRef" class="sentinel"></div>
    <p class="msg" v-if="errorMsg">{{ errorMsg }}</p>
    <p class="msg" v-else-if="loading">加载中…</p>
    <p class="msg" v-else-if="!dictEntries.length && loaded">
      {{ dictFilter.trim() ? '没有匹配的单词' : '词库暂无数据' }}
    </p>
    <p class="msg" v-else-if="dictEntries.length">共 {{ total }} 条，已加载 {{ dictEntries.length }} 条</p>
  </div>
</template>

<style scoped>
.sentinel {
  height: 1px;
}
</style>
