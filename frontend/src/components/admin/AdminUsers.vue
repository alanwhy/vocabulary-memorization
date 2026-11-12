<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { apiGet, apiPost, apiPut, apiDelete } from '@/api/client'
import { useAuthStore } from '@/stores/auth'
import { formatTime } from '@/utils/format'

const auth = useAuthStore()
const currentUserId = computed(() => auth.currentUser?.id)
const users = ref([])
const listLoading = ref(false)
const listError = ref('')
const createVisible = ref(false)
const newUser = reactive({ username: '', password: '', is_admin: false })
const creatingUser = ref(false)
const createError = ref('')
const dialogVisible = ref(false)
const dialogMode = ref('detail')
const selectedId = ref(null)
const detail = ref(null)
const detailLoading = ref(false)
const dialogError = ref('')
const saving = ref(false)
const resetting = ref(false)
const editDraft = reactive({ username: '', is_admin: false, disabled: false, unlimited: false, daily_limit_cny: '5.00' })
const editingSelf = computed(() => selectedId.value === currentUserId.value)
const generatedPassword = ref('')
const resetUsername = ref('')
const resetResultVisible = ref(false)

/** 读取用户列表，失败时保留当前内容并展示可重试提示。 */
async function loadUsers() {
  listLoading.value = true
  listError.value = ''
  try {
    users.value = await apiGet('/api/admin/users')
  } catch (e) {
    listError.value = e.message || '用户列表读取失败'
  } finally {
    listLoading.value = false
  }
}

/** 将后端金额字符串用于展示，保留小额费用的有效位数。
 * @param {string|null} value 人民币金额
 * @param {number} digits 最少显示的小数位数
 * @returns {string} 可读金额
 */
function money(value, digits = 4) {
  if (value == null) return '不限额'
  const [whole, fraction = ''] = String(value).split('.')
  return `¥${whole}.${fraction.padEnd(digits, '0')}`
}

/** 从最新详情初始化编辑字段，null 额度对应不限额。 */
function fillDraft() {
  const { user, budget } = detail.value
  Object.assign(editDraft, {
    username: user.username,
    is_admin: user.is_admin,
    disabled: user.disabled,
    unlimited: budget.daily_limit_cny == null,
    daily_limit_cny: budget.daily_limit_cny ?? '5.00',
  })
}

/** 打开详情或编辑弹窗并实时读取账号，忽略已切换账号的旧响应。
 * @param {object} user 选中的用户
 * @param {string} mode detail 或 edit
 */
async function openUser(user, mode) {
  selectedId.value = user.id
  dialogMode.value = mode
  dialogVisible.value = true
  detail.value = null
  dialogError.value = ''
  detailLoading.value = true
  const id = user.id
  try {
    const data = await apiGet(`/api/admin/users/${id}`)
    if (selectedId.value !== id || !dialogVisible.value) return
    detail.value = data
    fillDraft()
  } catch (e) {
    if (selectedId.value === id) dialogError.value = e.message || '用户详情读取失败'
  } finally {
    if (selectedId.value === id) detailLoading.value = false
  }
}

/** 保存账号与日额度，已用费用由服务器保留；修改自己后同步当前用户名。 */
async function saveUser() {
  if (saving.value || !detail.value) return
  const amount = editDraft.daily_limit_cny.trim()
  if (!editDraft.username.trim()) {
    dialogError.value = '请填写用户名'
    return
  }
  if (!editDraft.unlimited && (!/^(0|[1-9]\d{0,6})(\.\d{1,2})?$/.test(amount) || Number(amount) > 1000000)) {
    dialogError.value = '每日额度须为 0–1000000 元，最多两位小数'
    return
  }
  saving.value = true
  dialogError.value = ''
  try {
    const data = await apiPut(`/api/admin/users/${selectedId.value}`, {
      username: editDraft.username.trim(),
      is_admin: editDraft.is_admin,
      disabled: editDraft.disabled,
      daily_limit_cny: editDraft.unlimited ? null : amount,
    })
    if (data.user && editingSelf.value) Object.assign(auth.currentUser, data.user)
    ElMessage.success('用户信息已保存，每日额度立即生效')
    dialogVisible.value = false
    await loadUsers()
  } catch (e) {
    dialogError.value = e.message || '保存失败'
  } finally {
    saving.value = false
  }
}

/** 新增账号，普通用户默认每日五元，管理员默认不限额。 */
async function createUser() {
  if (creatingUser.value) return
  createError.value = ''
  creatingUser.value = true
  try {
    await apiPost('/api/admin/users', newUser)
    ElMessage.success(`已创建用户 ${newUser.username}`)
    Object.assign(newUser, { username: '', password: '', is_admin: false })
    createVisible.value = false
    await loadUsers()
  } catch (e) {
    createError.value = e.message || '创建失败'
  } finally {
    creatingUser.value = false
  }
}

/** 重置选中账号的密码，确认后展示一次性生成的密码；现有会话将失效。 */
async function resetPassword() {
  if (resetting.value || !detail.value) return
  const user = detail.value.user
  try {
    await ElMessageBox.confirm(`确定重置用户「${user.username}」的密码吗？该用户需要重新登录。`, '重置密码', {
      confirmButtonText: '重置', cancelButtonText: '取消', type: 'warning',
    })
  } catch { return }
  resetting.value = true
  try {
    const data = await apiPost(`/api/admin/users/${user.id}/reset-password`)
    resetUsername.value = user.username
    generatedPassword.value = data.password
    resetResultVisible.value = true
  } catch (e) {
    ElMessage.error(e.message || '重置失败')
  } finally {
    resetting.value = false
  }
}

/** 复制新密码，失败时用户仍可手动选择复制。 */
async function copyPassword() {
  try {
    await navigator.clipboard.writeText(generatedPassword.value)
    ElMessage.success('已复制')
  } catch { ElMessage.info('请选中新密码手动复制') }
}

/** 确认删除账号及个人词库，自己的账号不允许删除。
 * @param {object} user 待删除的用户
 */
async function deleteUser(user) {
  try {
    await ElMessageBox.confirm(`确定删除用户「${user.username}」吗？其个人单词记录会一并删除，且不可恢复。`, '删除用户', {
      confirmButtonText: '删除', cancelButtonText: '取消', type: 'warning',
    })
  } catch { return }
  try {
    await apiDelete(`/api/admin/users/${user.id}`)
    users.value = users.value.filter((item) => item.id !== user.id)
    ElMessage.success('用户已删除')
  } catch (e) { ElMessage.error(e.message || '删除失败') }
}

onMounted(loadUsers)
</script>

<template>
  <div class="users-header">
    <h2>用户管理</h2>
    <el-button type="primary" @click="createVisible = true; createError = ''">新增用户</el-button>
  </div>
  <div class="card">
    <el-alert v-if="listError" :title="listError" type="error" :closable="false" show-icon />
    <div class="table-scroll" v-loading="listLoading">
      <table class="users-table">
        <thead><tr><th>用户名</th><th>角色</th><th class="op-col">操作</th></tr></thead>
        <tbody>
          <tr v-for="user in users" :key="user.id">
            <td>{{ user.username }}</td>
            <td><span v-if="user.is_admin" class="badge">管理员</span><span v-else>普通用户</span></td>
            <td class="op-col">
              <button class="link-btn" @click="openUser(user, 'detail')">详情</button>
              <button class="link-btn" @click="openUser(user, 'edit')">编辑</button>
              <button class="link-btn danger" :disabled="user.id === currentUserId" @click="deleteUser(user)">删除</button>
            </td>
          </tr>
          <tr v-if="!listLoading && !users.length"><td colspan="3" class="users-empty">暂无用户</td></tr>
        </tbody>
      </table>
    </div>
    <el-button v-if="listError" size="small" @click="loadUsers">重新加载</el-button>
  </div>

  <el-dialog v-model="dialogVisible" :title="dialogMode === 'detail' ? '用户详情' : '编辑用户'" width="540px" style="max-width: 94vw" :close-on-click-modal="!saving" :show-close="!saving" :close-on-press-escape="!saving">
    <div v-if="detailLoading" class="users-empty">正在读取用户信息…</div>
    <el-alert v-if="dialogError" :title="dialogError" type="error" :closable="false" show-icon class="dialog-alert" />
    <template v-if="detail">
      <template v-if="dialogMode === 'detail'">
        <dl class="user-details">
          <dt>用户名</dt><dd>{{ detail.user.username }}</dd>
          <dt>角色</dt><dd>{{ detail.user.is_admin ? '管理员' : '普通用户' }}</dd>
          <dt>账号状态</dt><dd>{{ detail.user.disabled ? '已禁用' : '正常' }}</dd>
          <dt>创建时间</dt><dd>{{ formatTime(detail.user.created_at) }}</dd>
          <dt>最后登录</dt><dd>{{ formatTime(detail.user.last_login_at, '从未登录') }}</dd>
          <dt>单词数</dt><dd>{{ detail.user.word_count }}</dd>
        </dl>
        <div class="budget-panel">
          <div class="budget-title">今日 AI 额度 <span>{{ detail.budget.date }}</span></div>
          <dl class="user-details">
            <dt>每日额度</dt><dd>{{ money(detail.budget.daily_limit_cny, 2) }}</dd>
            <dt>今日已用</dt><dd>{{ money(detail.budget.spent_cny) }}</dd>
            <dt>请求预留</dt><dd>{{ money(detail.budget.reserved_cny) }}</dd>
            <dt>剩余额度</dt><dd>{{ money(detail.budget.remaining_cny) }}</dd>
          </dl>
          <p v-if="detail.budget.pending_requests" class="budget-note">有 {{ detail.budget.pending_requests }} 笔请求正在处理或等待费用核算，预留金额会占用额度。</p>
        </div>
      </template>
      <el-form v-else label-position="top" @submit.prevent="saveUser">
        <el-form-item label="用户名"><el-input v-model="editDraft.username" maxlength="64" :disabled="saving" /></el-form-item>
        <div class="edit-row">
          <el-form-item label="角色">
            <el-select v-model="editDraft.is_admin" :disabled="editingSelf || saving">
              <el-option label="普通用户" :value="false" /><el-option label="管理员" :value="true" />
            </el-select>
          </el-form-item>
          <el-form-item label="账号状态">
            <el-select v-model="editDraft.disabled" :disabled="editingSelf || saving">
              <el-option label="正常" :value="false" /><el-option label="已禁用" :value="true" />
            </el-select>
          </el-form-item>
        </div>
        <el-form-item label="每日 AI 额度">
          <div class="quota-input">
            <el-input v-model="editDraft.daily_limit_cny" :disabled="editDraft.unlimited || saving" inputmode="decimal" placeholder="例如 5.00"><template #append>元 / 天</template></el-input>
            <el-checkbox v-model="editDraft.unlimited" :disabled="saving">不限额</el-checkbox>
          </div>
        </el-form-item>
        <p class="budget-note">今日已用 {{ money(detail.budget.spent_cny) }}，请求预留 {{ money(detail.budget.reserved_cny) }}。调整额度立即生效，已用金额保留；设为 0 元可暂停新增付费调用。</p>
        <div class="password-action"><span>账号密码</span><el-button :loading="resetting" :disabled="saving" @click="resetPassword">重置密码</el-button></div>
      </el-form>
      <p class="budget-note billing-note">按北京时间每天零点更新额度。包含模型、搜索和网页抓取，不含语音。费用按官方费率估算，节假日高峰窗口采用保守价格。</p>
    </template>
    <template #footer>
      <el-button :disabled="saving" @click="dialogVisible = false">{{ dialogMode === 'detail' ? '关闭' : '取消' }}</el-button>
      <el-button v-if="!detail && !detailLoading" @click="openUser({ id: selectedId }, dialogMode)">重新加载</el-button>
      <el-button v-if="dialogMode === 'detail' && detail" type="primary" @click="dialogMode = 'edit'; fillDraft()">编辑用户</el-button>
      <el-button v-if="dialogMode === 'edit' && detail" type="primary" :loading="saving" :disabled="resetting" @click="saveUser">保存</el-button>
    </template>
  </el-dialog>

  <el-dialog v-model="createVisible" title="新增用户" width="460px" style="max-width: 94vw" :close-on-click-modal="!creatingUser" :show-close="!creatingUser" :close-on-press-escape="!creatingUser">
    <el-form label-position="top" @submit.prevent="createUser">
      <el-form-item label="用户名"><el-input v-model="newUser.username" maxlength="64" :disabled="creatingUser" /></el-form-item>
      <el-form-item label="密码"><el-input v-model="newUser.password" type="password" show-password placeholder="至少 6 位" :disabled="creatingUser" /></el-form-item>
      <el-form-item><el-checkbox v-model="newUser.is_admin" :disabled="creatingUser">设为管理员</el-checkbox></el-form-item>
      <p class="budget-note">{{ newUser.is_admin ? '管理员默认不限额，创建后可在编辑中调整。' : '普通用户默认每天 5 元，创建后可在编辑中调整。' }}</p>
      <el-alert v-if="createError" :title="createError" type="error" :closable="false" show-icon />
    </el-form>
    <template #footer><el-button :disabled="creatingUser" @click="createVisible = false">取消</el-button><el-button type="primary" :loading="creatingUser" @click="createUser">创建</el-button></template>
  </el-dialog>

  <el-dialog v-model="resetResultVisible" title="密码已重置" width="380px" style="max-width: 92vw">
    <p>用户「{{ resetUsername }}」的新密码：</p>
    <p><code class="generated-password">{{ generatedPassword }}</code> <el-button size="small" @click="copyPassword">复制</el-button></p>
    <template #footer><el-button @click="resetResultVisible = false">关闭</el-button></template>
  </el-dialog>
</template>

<style scoped>
.users-header { display: flex; align-items: center; justify-content: space-between; gap: 16px; margin-bottom: 16px; }
.users-header h2 { margin: 0; }
.users-table { width: 100%; }
.users-table th:last-child, .users-table td:last-child { width: 180px; white-space: nowrap; }
.users-empty { padding: 28px; text-align: center; color: var(--el-text-color-secondary); }
.user-details { display: grid; grid-template-columns: 100px minmax(0, 1fr); gap: 12px 20px; margin: 0; }
.user-details dt { color: var(--el-text-color-secondary); }
.user-details dd { margin: 0; overflow-wrap: anywhere; }
.budget-panel { margin-top: 24px; padding: 18px; border: 1px solid var(--el-border-color-lighter); border-radius: 10px; background: var(--el-fill-color-extra-light); }
.budget-title { display: flex; align-items: center; justify-content: space-between; margin-bottom: 18px; font-weight: 600; }
.budget-title span { color: var(--el-text-color-secondary); font-size: 12px; font-weight: 400; }
.budget-note { margin: 10px 0 0; color: var(--el-text-color-secondary); font-size: 12px; line-height: 1.7; }
.billing-note { margin-top: 18px; }
.edit-row { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; }
.quota-input { display: flex; align-items: center; gap: 16px; width: 100%; }
.password-action { display: flex; justify-content: space-between; align-items: center; margin-top: 22px; padding-top: 18px; border-top: 1px solid var(--el-border-color-lighter); }
.dialog-alert { margin-bottom: 16px; }
@media (max-width: 480px) { .user-details { grid-template-columns: 84px minmax(0, 1fr); gap: 10px; } .quota-input { gap: 10px; } }
</style>
