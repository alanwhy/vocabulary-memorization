<script setup>
import { RouterView } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import AppTopbar from '@/components/layout/AppTopbar.vue'

const auth = useAuthStore()
</script>

<template>
  <div v-if="!auth.checked" class="empty">加载中...</div>
  <div v-else>
    <AppTopbar v-if="auth.isAuthenticated" />
    <RouterView />
  </div>
</template>
