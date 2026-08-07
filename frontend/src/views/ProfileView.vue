<script setup>
import { onMounted, ref } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { apiGet } from '@/api/client'
import { formatTime } from '@/utils/format'
import ChangePasswordDialog from '@/components/profile/ChangePasswordDialog.vue'

const auth = useAuthStore()

// 录入单词数取 total_all_words：含已归档，口径是「这个账号一共录进来多少个词」
const wordCount = ref(null)

onMounted(async () => {
  try {
    const stats = await apiGet('/api/stats')
    wordCount.value = stats.total_all_words
  } catch {
    // 拉不到就显示占位符，不影响个人中心其他信息
  }
})
</script>

<template>
  <div>
    <h1>个人中心</h1>

    <div class="card" v-if="auth.currentUser">
      <el-descriptions :column="1" border>
        <el-descriptions-item label="用户名">{{ auth.currentUser.username }}</el-descriptions-item>
        <el-descriptions-item label="角色">{{ auth.currentUser.is_admin ? '超管' : '普通用户' }}</el-descriptions-item>
        <el-descriptions-item label="注册时间">{{ formatTime(auth.currentUser.created_at) }}</el-descriptions-item>
        <el-descriptions-item label="最后登录时间">{{ formatTime(auth.currentUser.last_login_at) }}</el-descriptions-item>
        <el-descriptions-item label="录入单词数">{{ wordCount === null ? '—' : wordCount }}</el-descriptions-item>
      </el-descriptions>
    </div>

    <ChangePasswordDialog />
  </div>
</template>
