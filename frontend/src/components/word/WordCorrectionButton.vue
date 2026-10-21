<script setup>
// 背单词/闪卡共享纠错入口：必填反馈 -> 完整重查 -> corrected 局部刷新，不改变复习进度。
import { ref } from 'vue'
import { ElMessage } from 'element-plus'
import { apiPost } from '@/api/client'

const props = defineProps({
  word: { type: Object, required: true },
  disabled: { type: Boolean, default: false },
})
const emit = defineEmits(['corrected', 'busy'])
const visible = ref(false)
const reason = ref('')
const submitting = ref(false)
const error = ref('')

function open() {
  reason.value = ''
  error.value = ''
  visible.value = true
}

async function confirm() {
  if (submitting.value) return
  const feedback = reason.value.trim()
  if (!feedback || [...feedback].length > 500) {
    error.value = '请填写 1–500 字的错误原因'
    return
  }
  const id = props.word.id
  submitting.value = true
  emit('busy', true)
  error.value = ''
  try {
    const fresh = await apiPost(`/api/words/${id}/correct`, { reason: feedback })
    emit('corrected', fresh)
    visible.value = false
    ElMessage.success('单词内容已重新查询并更新')
  } catch (e) {
    error.value = e.message || '纠错查询失败，请重试'
  } finally {
    submitting.value = false
    emit('busy', false)
  }
}
</script>

<template>
  <button type="button" class="correct-btn" :disabled="disabled || word.translating || submitting" @click.stop="open">纠错</button>
  <el-dialog
    v-model="visible"
    :title="`纠错：${word.display_word}`"
    width="min(480px, 92vw)"
    append-to-body
    :close-on-click-modal="!submitting"
    :close-on-press-escape="!submitting"
    :show-close="!submitting"
    @click.stop
  >
    <p class="description">请说明错误原因，确定后会重新查询释义、例句、词根词缀、派生词等全部内容。</p>
    <label :for="`correction-reason-${word.id}`">错误原因</label>
    <el-input :id="`correction-reason-${word.id}`" v-model="reason" type="textarea" :rows="4" maxlength="500" show-word-limit :disabled="submitting" placeholder="例如：过去式被列成了形近词，例句翻译有误" />
    <p v-if="error" class="error" role="alert">{{ error }}</p>
    <template #footer>
      <el-button :disabled="submitting" @click="visible = false">取消</el-button>
      <el-button type="primary" :loading="submitting" @click="confirm">{{ submitting ? '重新查询中' : '确定' }}</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.correct-btn { border: 1px solid var(--border); border-radius: 6px; background: var(--card-bg); color: var(--accent); cursor: pointer; padding: 5px 12px; }
.correct-btn:disabled { opacity: 0.5; cursor: not-allowed; }
.description { color: var(--muted); line-height: 1.6; margin-top: 0; }
label { display: block; margin-bottom: 8px; }
.error { color: var(--danger); }
</style>
