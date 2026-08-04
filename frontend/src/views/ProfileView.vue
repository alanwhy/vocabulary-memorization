<script setup>
import { useAuthStore } from '@/stores/auth'
import ChangePasswordDialog from '@/components/profile/ChangePasswordDialog.vue'

const auth = useAuthStore()

function formatTime(iso) {
  const d = new Date(iso)
  const pad = (n) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}
</script>

<template>
  <div>
    <h1>个人中心</h1>

    <div class="card" v-if="auth.currentUser">
      <el-descriptions :column="1" border>
        <el-descriptions-item label="用户名">{{ auth.currentUser.username }}</el-descriptions-item>
        <el-descriptions-item label="角色">{{ auth.currentUser.is_admin ? '超管' : '普通用户' }}</el-descriptions-item>
        <el-descriptions-item label="注册时间">{{ formatTime(auth.currentUser.created_at) }}</el-descriptions-item>
      </el-descriptions>
    </div>

    <ChangePasswordDialog />
  </div>
</template>
