<script setup>
import { reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { apiPut, apiPost } from '@/api/client'

const visible = ref(false)
const pwForm = reactive({ old_password: '', new_password: '', confirm_password: '' })
const changingPassword = ref(false)
const resetting = ref(false)

function resetPwForm() {
  pwForm.old_password = ''
  pwForm.new_password = ''
  pwForm.confirm_password = ''
}

async function submitChangePassword() {
  if (changingPassword.value) return
  if (!pwForm.old_password || !pwForm.new_password) {
    ElMessage.error('请填写完整')
    return
  }
  if (pwForm.new_password.length < 6) {
    ElMessage.error('新密码长度至少 6 位')
    return
  }
  if (pwForm.new_password !== pwForm.confirm_password) {
    ElMessage.error('两次输入的新密码不一致')
    return
  }
  changingPassword.value = true
  try {
    await apiPut('/api/me/password', {
      old_password: pwForm.old_password,
      new_password: pwForm.new_password,
    })
    ElMessage.success('密码已修改')
    visible.value = false
  } catch (e) {
    ElMessage.error(e.message || '修改失败')
  } finally {
    changingPassword.value = false
  }
}

// 重置次数：把当前用户所有单词的背诵次数重置为 1，二次确认后调用后端
async function resetCounts() {
  try {
    await ElMessageBox.confirm('确定将所有单词的背诵次数重置为 1 吗？该操作不可撤销。', '提示', {
      confirmButtonText: '重置',
      cancelButtonText: '取消',
      type: 'warning',
    })
  } catch {
    return
  }
  resetting.value = true
  try {
    await apiPost('/api/words/reset-counts')
    ElMessage.success('已重置所有单词的次数')
  } catch (e) {
    ElMessage.error(e.message || '重置失败，请重试')
  } finally {
    resetting.value = false
  }
}
</script>

<template>
  <div class="card">
    <div class="action-row">
      <el-button type="primary" @click="visible = true">修改密码</el-button>
      <el-button type="danger" :loading="resetting" @click="resetCounts">重置次数</el-button>
    </div>
  </div>

  <el-dialog v-model="visible" title="修改密码" width="380px" style="max-width: 92vw" @closed="resetPwForm">
    <el-form label-position="top">
      <el-form-item label="原密码">
        <el-input type="password" v-model="pwForm.old_password" show-password />
      </el-form-item>
      <el-form-item label="新密码">
        <el-input type="password" v-model="pwForm.new_password" placeholder="至少 6 位" show-password />
      </el-form-item>
      <el-form-item label="确认新密码">
        <el-input
          type="password"
          v-model="pwForm.confirm_password"
          show-password
          @keyup.enter="submitChangePassword"
        />
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="visible = false">取消</el-button>
      <el-button type="primary" :loading="changingPassword" @click="submitChangePassword">提交</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.action-row {
  display: flex;
  gap: 12px;
  align-items: center;
}
</style>
