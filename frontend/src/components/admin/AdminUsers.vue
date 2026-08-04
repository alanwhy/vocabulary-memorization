<script setup>
import { onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { apiGet, apiPost } from '@/api/client'

const users = ref([])
const newUser = reactive({ username: '', password: '', is_admin: false })
const creatingUser = ref(false)
const userMsgError = ref('')
const userMsgOk = ref('')
const generatedPassword = ref('')
const resetUsername = ref('')
const resetResultVisible = ref(false)

function formatTime(iso) {
  const d = new Date(iso)
  const pad = (n) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

async function loadUsers() {
  users.value = await apiGet('/api/admin/users')
}

async function resetPassword(u) {
  try {
    await ElMessageBox.confirm(`确定重置用户「${u.username}」的密码吗？`, '提示', {
      confirmButtonText: '重置',
      cancelButtonText: '取消',
      type: 'warning',
    })
  } catch {
    return
  }
  try {
    const data = await apiPost(`/api/admin/users/${u.id}/reset-password`)
    resetUsername.value = u.username
    generatedPassword.value = data.password
    resetResultVisible.value = true
  } catch (e) {
    ElMessage.error(e.message || '重置失败')
  }
}

async function copyPassword() {
  try {
    await navigator.clipboard.writeText(generatedPassword.value)
  } catch {
    // 剪贴板 API 不可用时忽略，管理员仍可从页面上手动选中复制
  }
}

async function createUser() {
  userMsgError.value = ''
  userMsgOk.value = ''
  if (creatingUser.value) return
  creatingUser.value = true
  try {
    const data = await apiPost('/api/admin/users', newUser)
    users.value.push(data)
    userMsgOk.value = `已创建用户 ${data.username}`
    newUser.username = ''
    newUser.password = ''
    newUser.is_admin = false
  } catch (e) {
    userMsgError.value = e.message || '创建失败'
  } finally {
    creatingUser.value = false
  }
}

onMounted(loadUsers)
</script>

<template>
  <h2>用户管理</h2>
  <div class="card">
    <table>
      <thead>
        <tr>
          <th>用户名</th>
          <th>角色</th>
          <th>创建时间</th>
          <th>操作</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="u in users" :key="u.id">
          <td>{{ u.username }}</td>
          <td><span class="badge" v-if="u.is_admin">超管</span><span v-else>普通用户</span></td>
          <td>{{ formatTime(u.created_at) }}</td>
          <td><button class="link-btn" @click="resetPassword(u)">重置密码</button></td>
        </tr>
      </tbody>
    </table>

    <label>用户名</label>
    <input type="text" v-model="newUser.username" placeholder="新用户的用户名" />
    <label>密码</label>
    <input type="password" v-model="newUser.password" placeholder="至少 6 位" />
    <div class="checkbox-row">
      <input type="checkbox" id="isAdminCheckbox" v-model="newUser.is_admin" />
      <label for="isAdminCheckbox" style="margin: 0">设为超管</label>
    </div>
    <button class="primary" :disabled="creatingUser" @click="createUser">新增用户</button>
    <p class="msg" :class="{ error: !!userMsgError, ok: !!userMsgOk }">{{ userMsgError || userMsgOk }}</p>
  </div>

  <el-dialog v-model="resetResultVisible" title="密码已重置" width="380px" style="max-width: 92vw">
    <p>用户「{{ resetUsername }}」的新密码：</p>
    <p>
      <code class="generated-password">{{ generatedPassword }}</code>
      <el-button size="small" @click="copyPassword">复制</el-button>
    </p>
    <template #footer>
      <el-button @click="resetResultVisible = false">关闭</el-button>
    </template>
  </el-dialog>
</template>
