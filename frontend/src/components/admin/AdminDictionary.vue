<script setup>
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { apiGet, apiDelete, apiPost } from '@/api/client'
import { usePaginatedList } from '@/composables/usePaginatedList'
import { useInfiniteScroll } from '@/composables/useInfiniteScroll'
import { formatTime } from '@/utils/format'
import { countBadgeClass } from '@/utils/reviewLevel'

const dictFilter = ref('')
const statusFilter = ref('')

// 过滤交给后端：词库可能有上万条，本地过滤前提是先把整张表拉下来。
// 后端在 word_key 和 senses[].translation 上同时做模糊匹配，所以同一个输入框既支持
// 「按单词过滤」也支持「按释义过滤」；status 按释义有无过滤（暂无释义/已有释义）。
const list = usePaginatedList((page, limit) =>
  apiGet(
    `/api/admin/dictionary?keyword=${encodeURIComponent(dictFilter.value.trim())}&status=${encodeURIComponent(statusFilter.value)}&page=${page}&limit=${limit}`,
  ),
)
const { items: dictEntries, total, totalAll, loading, hasMore, loaded, errorMsg, reset, loadMore } = list

const sentinelRef = ref(null)
useInfiniteScroll(sentinelRef, loadMore)

// 输入一个字符就打一次接口太浪费，停手 300ms 后再回到第一页重查
let filterTimer = null
watch([dictFilter, statusFilter], () => {
  if (filterTimer) clearTimeout(filterTimer)
  filterTimer = setTimeout(reset, 300)
})
onUnmounted(() => {
  if (filterTimer) clearTimeout(filterTimer)
})

// 多选状态：selectedKeys 存勾选的 word_key 集合；allChecked 是表头全选框的当前状态，
// 跟实际选中数分开存放，避免渲染时根据 selectedKeys 重算导致 indeterminate 抖动。
const selectedKeys = ref(new Set())
const allChecked = ref(false)

function isSelected(d) {
  return selectedKeys.value.has(d.word_key)
}

function toggleOne(d, checked) {
  if (checked) {
    selectedKeys.value.add(d.word_key)
  } else {
    selectedKeys.value.delete(d.word_key)
  }
  selectedKeys.value = new Set(selectedKeys.value)
}

function toggleAll(checked) {
  allChecked.value = checked
  if (checked) {
    selectedKeys.value = new Set(dictEntries.value.map((d) => d.word_key))
  } else {
    selectedKeys.value = new Set()
  }
}

const hasSelection = computed(() => selectedKeys.value.size > 0)

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

async function batchDelete() {
  const keys = Array.from(selectedKeys.value)
  if (!keys.length) return
  try {
    await ElMessageBox.confirm(
      `确定批量删除选中的 ${keys.length} 条词库缓存吗？`,
      '提示',
      {
        confirmButtonText: '删除',
        cancelButtonText: '取消',
        type: 'warning',
      },
    )
  } catch {
    return
  }
  try {
    await apiPost('/api/admin/dictionary/batch-delete', { word_keys: keys })
    const keySet = new Set(keys)
    list.removeItem((x) => keySet.has(x.word_key))
    selectedKeys.value = new Set()
    allChecked.value = false
    ElMessage.success(`已删除 ${keys.length} 条`)
  } catch {
    ElMessage.error('批量删除失败，请重试')
  }
}

// 手动重新查询：针对「暂无释义」的词条，管理员判断可能只是接口超时导致的假阴性时，
// 主动触发一次查词并写回词库缓存。一次只允许一条在途，避免重复点击。
const retryingKey = ref('')
async function retryDictEntry(d) {
  if (retryingKey.value) return
  retryingKey.value = d.word_key
  try {
    await apiPost('/api/admin/dictionary/retry', { word_key: d.word_key })
    ElMessage.success(`已重新查询「${d.display_word}」`)
    // 重新拉第一页：在「暂无释义」筛选下，重查成功的词条应从列表里消失
    await reset()
  } catch (e) {
    ElMessage.error(e.message || '重新查询失败，请重试')
  } finally {
    retryingKey.value = ''
  }
}

onMounted(reset)
</script>

<template>
  <h2>词库管理 <span class="total-count">共 {{ totalAll }} 个单词</span></h2>
  <div class="card">
    <div class="toolbar">
      <a class="export-btn" href="/api/admin/dictionary/export">导出 CSV</a>
      <input type="text" v-model="dictFilter" placeholder="按单词或释义模糊搜索" />
      <select v-model="statusFilter" class="status-filter" aria-label="按释义状态筛选">
        <option value="">全部</option>
        <option value="no_definition">暂无释义</option>
        <option value="has_definition">已有释义</option>
      </select>
      <button class="batch-delete-btn" :disabled="!hasSelection" @click="batchDelete">
        批量删除{{ hasSelection ? `（${selectedKeys.size}）` : '' }}
      </button>
    </div>
    <table style="margin-top: 12px">
      <thead>
        <tr>
          <th class="check-col">
            <input
              type="checkbox"
              :checked="allChecked"
              :indeterminate.prop="!allChecked && hasSelection"
              @change="toggleAll($event.target.checked)"
            />
          </th>
          <th>单词</th>
          <th>释义</th>
          <th>出现次数</th>
          <th>最后更新时间</th>
          <th>操作</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="d in dictEntries" :key="d.word_key">
          <td class="check-col">
            <input
              type="checkbox"
              :checked="isSelected(d)"
              @change="toggleOne(d, $event.target.checked)"
            />
          </td>
          <td>{{ d.display_word }}</td>
          <td>
            <div class="senses" v-if="d.senses && d.senses.length">
              <div class="phonetic" v-if="d.senses[0].phonetic">{{ d.senses[0].phonetic }}</div>
              <div class="sense" v-for="(s, idx) in d.senses" :key="idx">
                <span class="pos" v-if="s.pos">{{ s.pos }}</span>
                <span class="translation">{{ s.translation }}</span>
              </div>
            </div>
            <span v-else>暂无释义</span>
          </td>
          <td><span :class="countBadgeClass(d.occurrence_count)">×{{ d.occurrence_count }}</span></td>
          <td>{{ formatTime(d.last_updated_at) }}</td>
          <td>
            <button class="link-btn" v-if="!d.senses || !d.senses.length" @click="retryDictEntry(d)">
              {{ retryingKey === d.word_key ? '查询中…' : '重新查询' }}
            </button>
            <button class="link-btn danger" @click="deleteDictEntry(d)">删除</button>
          </td>
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
.total-count {
  font-size: 13px;
  font-weight: 400;
  color: var(--muted);
  margin-left: 8px;
}
.toolbar {
  display: flex;
  gap: 8px;
  align-items: center;
  flex-wrap: wrap;
}
.toolbar input[type='text'] {
  flex: 1;
  min-width: 180px;
  padding: 8px 12px;
  border: 1px solid var(--border);
  border-radius: 8px;
  background: var(--card-bg);
  color: var(--text);
  outline: none;
}
.toolbar input[type='text']:focus {
  border-color: var(--accent);
}
.toolbar .status-filter {
  padding: 8px 12px;
  border: 1px solid var(--border);
  border-radius: 8px;
  background: var(--card-bg);
  color: var(--text);
  outline: none;
}
.toolbar .status-filter:focus {
  border-color: var(--accent);
}
.toolbar .batch-delete-btn {
  padding: 8px 14px;
  font-size: 13px;
  border: 1px solid var(--danger);
  border-radius: 8px;
  background: transparent;
  color: var(--danger);
  cursor: pointer;
}
.toolbar .batch-delete-btn:disabled {
  border-color: var(--border);
  color: var(--muted);
  cursor: default;
}
.phonetic {
  font-size: 12px;
  color: var(--muted);
}
.check-col {
  width: 36px;
  text-align: center;
}
.sentinel {
  height: 1px;
}
</style>