<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { apiGet, apiPost } from '@/api/client'

const props = defineProps({ disabled: Boolean })
const emit = defineEmits(['busy', 'synced'])
const task = ref({ status: 'idle' })
const checking = ref(true)
const starting = ref(false)
const errorText = ref('')
const busy = computed(() => checking.value || starting.value || task.value.status === 'running')
let active = true
let pollTimer = null
let pendingPreviousId = null

const message = computed(() => {
  if (errorText.value) return errorText.value
  if (checking.value) return '正在读取同步状态…'
  if (starting.value) return '正在准备同步…'
  if (task.value.status === 'running') return '同步中，请稍候。完成后会显示结果，可稍后回来查看。'
  if (task.value.status === 'completed') return `同步完成，新增 ${task.value.added_count} 个单词。`
  if (task.value.status === 'failed') return task.value.message || '同步失败，请重试。'
  return '仅同步有释义的单词，已有次数保持不变。'
})

watch(busy, (value) => emit('busy', value), { immediate: true })

/** 安排下一次查询，避免重叠请求；离开个人中心后不再轮询。 */
function schedulePoll() {
  if (!active) return
  if (pollTimer) clearTimeout(pollTimer)
  pollTimer = setTimeout(() => {
    pollTimer = null
    refreshStatus()
  }, 2000)
}

/**
 * 更新本人任务状态；响应丢失时核对任务 ID，避免把上次结果当成本次成功。
 * @param {object} data 后端返回的本人任务快照。
 */
function applyStatus(data) {
  if (!active) return
  const wasWaiting = starting.value || task.value.status === 'running'
  if (pendingPreviousId !== null && data.status !== 'running' &&
      (data.status === 'idle' || data.task_id === pendingPreviousId)) {
    pendingPreviousId = null
    task.value = data
    errorText.value = '同步请求未确认启动，请重新同步。'
    return
  }
  pendingPreviousId = null
  errorText.value = ''
  task.value = data
  if (data.status === 'running') {
    schedulePoll()
  } else if (data.status === 'idle' && wasWaiting) {
    errorText.value = '同步状态已失效，请重新同步。'
  } else if (data.status === 'completed' && wasWaiting) {
    emit('synced')
    ElMessage.success(`同步完成，新增 ${data.added_count} 个单词`)
  }
}

/**
 * 查询服务端真实状态；网络故障继续等待，不将其误判为任务失败。
 * @returns {Promise<void>} 完成本轮查询或安排重试。
 */
async function refreshStatus() {
  try {
    applyStatus(await apiGet('/api/me/sync-words'))
  } catch (e) {
    if (!active) return
    errorText.value = task.value.status === 'running'
      ? '暂时无法读取结果，正在重试，请稍候…'
      : (e.message || '读取同步状态失败，可以重新发起同步。')
    if (task.value.status === 'running') schedulePoll()
  } finally {
    if (active) checking.value = false
  }
}

/**
 * 确认后仅为当前登录账号发起异步同步，并持续查询完成结果。
 * @returns {Promise<void>} 请求已提交、取消或转入状态确认。
 */
async function startSync() {
  if (busy.value || props.disabled) return
  starting.value = true
  try {
    try {
      await ElMessageBox.confirm(
        '将母库有释义的单词同步到你的账号，新增单词次数为 1，已有次数和学习记录保留。同步需要等待一段时间。',
        '同步所有单词',
        { confirmButtonText: '开始同步', cancelButtonText: '取消', type: 'info' },
      )
    } catch {
      return
    }
    const previousId = task.value.task_id || ''
    try {
      applyStatus(await apiPost('/api/me/sync-words'))
    } catch (e) {
      if (!active) return
      pendingPreviousId = previousId
      task.value = { ...task.value, status: 'running' }
      errorText.value = e.message || '正在确认同步请求…'
      await refreshStatus()
    }
  } finally {
    if (active) starting.value = false
  }
}

onMounted(refreshStatus)
onBeforeUnmount(() => {
  active = false
  if (pollTimer) clearTimeout(pollTimer)
})
</script>

<template>
  <div class="sync-action">
    <el-button type="success" :loading="busy" :disabled="disabled || busy" @click="startSync">
      {{ task.status === 'running' ? '同步中，请稍候' : '同步所有单词' }}
    </el-button>
    <p class="sync-state" :class="{ error: errorText || task.status === 'failed' }" role="status" aria-live="polite">
      {{ message }}
    </p>
  </div>
</template>

<style scoped>
.sync-action { max-width: 360px; }
.sync-state { margin: 8px 0 0; color: var(--el-text-color-secondary); font-size: 13px; line-height: 1.6; }
.sync-state.error { color: var(--el-color-danger); }
</style>
