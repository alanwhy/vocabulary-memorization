<script setup>
import { RouterLink } from 'vue-router'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const router = useRouter()

async function logout() {
  await auth.logout()
  router.push('/')
}
</script>

<template>
  <div class="topbar">
    <RouterLink to="/profile">{{ auth.currentUser?.username }}</RouterLink>
    <span>
      <RouterLink to="/">背单词</RouterLink>
      <RouterLink to="/archive">归档</RouterLink>
      <RouterLink to="/stats">统计</RouterLink>
      <RouterLink v-if="auth.isAdmin" to="/admin">后台管理</RouterLink>
      <button @click="logout">退出登录</button>
    </span>
  </div>
</template>
