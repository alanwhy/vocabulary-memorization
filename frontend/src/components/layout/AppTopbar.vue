<script setup>
import { RouterLink, useRoute } from 'vue-router'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { useTheme } from '@/composables/useTheme'

const auth = useAuthStore()
const router = useRouter()
const route = useRoute()
const { theme, toggleTheme } = useTheme()

async function logout() {
  await auth.logout()
  router.push('/')
}
</script>

<template>
  <div class="topbar">
    <span>
      <RouterLink to="/profile">{{ auth.currentUser?.username }}</RouterLink>
      <button
        class="theme-btn"
        :title="theme === 'dark' ? '切换到浅色主题' : '切换到深色主题'"
        :aria-label="theme === 'dark' ? '切换到浅色主题' : '切换到深色主题'"
        @click="toggleTheme"
      >
        {{ theme === 'dark' ? '☀️' : '🌙' }}
      </button>
    </span>
    <span>
      <RouterLink v-if="route.path !== '/'" to="/">背单词</RouterLink>
      <RouterLink v-if="route.path !== '/flashcards'" to="/flashcards">闪卡</RouterLink>
      <RouterLink v-if="route.path !== '/archive'" to="/archive">归档</RouterLink>
      <RouterLink v-if="route.path !== '/stats'" to="/stats">统计</RouterLink>
      <RouterLink v-if="auth.isAdmin && route.path !== '/admin'" to="/admin">后台管理</RouterLink>
      <button @click="logout">退出登录</button>
    </span>
  </div>
</template>

<style scoped>
/* 只留图标，padding 收窄成近似正方形，免得按钮比旁边的用户名还宽 */
.theme-btn {
  padding: 4px 7px;
  line-height: 1.2;
}
</style>
