<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { apiGet, apiPost, apiDelete } from '@/api/client'
import { useAuthStore } from '@/stores/auth'
import { formatTime } from '@/utils/format'

const auth = useAuthStore()
const currentUserId = computed(() => auth.currentUser?.id)

const users = ref([])
const newUser = reactive({ username: '', password: '', is_admin: false })
const creatingUser = ref(false)
const userMsgError = ref('')
const userMsgOk = ref('')
const generatedPassword = ref('')
const resetUsername = ref('')
const resetResultVisible = ref(false)

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
    // 创建接口返回的是 User，没有列表里的 word_count 字段，补个 0 免得这一列显示空白
    users.value.push({ ...data, word_count: 0 })
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

async function toggleDisabled(u) {
  const action = u.disabled ? '启用' : '禁用'
  try {
    await ElMessageBox.confirm(`确定${action}用户「${u.username}」吗？`, '提示', {
      confirmButtonText: action,
      cancelButtonText: '取消',
      type: 'warning',
    })
  } catch {
    return
  }
  try {
    await apiPost(`/api/admin/users/${u.id}/disable`, { disabled: !u.disabled })
    u.disabled = !u.disabled
    ElMessage.success(`已${action}用户「${u.username}」`)
  } catch (e) {
    ElMessage.error(e.message || `${action}失败`)
  }
}

async function deleteUser(u) {
  try {
    await ElMessageBox.confirm(
      `确定删除用户「${u.username}」吗？其名下所有单词记录也会一并删除，且不可恢复。`,
      '提示',
      { confirmButtonText: '删除', cancelButtonText: '取消', type: 'warning' },
    )
  } catch {
    return
  }
  try {
    await apiDelete(`/api/admin/users/${u.id}`)
    users.value = users.value.filter((x) => x.id !== u.id)
    ElMessage.success(`已删除用户「${u.username}」`)
  } catch (e) {
    ElMessage.error(e.message || '删除失败')
  }
}

onMounted(loadUsers)
</script>

<template>
  <h2>用户管理</h2>
  <div class="card">
    <div class="table-scroll">
      <table>
        <thead>
          <tr>
            <th>用户名</th>
            <th>角色</th>
            <th>状态</th>
            <th>创建时间</th>
            <th>最后登录</th>
            <th>单词数</th>
            <th class="op-col">操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="u in users" :key="u.id">
            <td>{{ u.username }}</td>
            <td><span class="badge" v-if="u.is_admin">超管</span><span v-else>普通用户</span></td>
            <td><span class="badge" :class="{ disabled: u.disabled }">{{ u.disabled ? '已禁用' : '正常' }}</span></td>
            <td class="nowrap">{{ formatTime(u.created_at) }}</td>
            <td class="nowrap">{{ formatTime(u.last_login_at, '从未登录') }}</td>
            <td>{{ u.word_count }}</td>
            <td class="op-col">
              <button class="link-btn" @click="resetPassword(u)">重置密码</button>
              <button class="link-btn" :disabled="u.id === currentUserId" @click="toggleDisabled(u)">
                {{ u.disabled ? '启用' : '禁用' }}
              </button>
              <button class="link-btn danger" :disabled="u.id === currentUserId" @click="deleteUser(u)">删除</button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

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
