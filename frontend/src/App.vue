<script setup>
import { watch } from 'vue'
import { RouterView } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import AppTopbar from '@/components/layout/AppTopbar.vue'
import WordLookupTooltip from '@/components/word/WordLookupTooltip.vue'

const auth = useAuthStore()

// 统计页引入了 echarts + echarts-wordcloud，懒加载 chunk 约 1MB，首次点「统计」会卡在下载上。
// 登录后等浏览器空闲时预加载这个 chunk，让用户点进去时模块已就绪，不用等下载解析。
let statsPreloaded = false
function preloadStats() {
  if (statsPreloaded) return
  statsPreloaded = true
  if ('requestIdleCallback' in window) {
    window.requestIdleCallback(() => import('@/views/StatsView.vue'), { timeout: 2000 })
  } else {
    setTimeout(() => import('@/views/StatsView.vue'), 2000)
  }
}

watch(
  () => auth.isAuthenticated,
  (isAuth) => {
    if (isAuth) preloadStats()
  },
  { immediate: true },
)
</script>

<template>
  <div v-if="!auth.checked" class="empty">加载中...</div>
  <div v-else>
    <AppTopbar v-if="auth.isAuthenticated" />
    <RouterView />
    <WordLookupTooltip />
  </div>
</template>
