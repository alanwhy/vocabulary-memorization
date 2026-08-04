import { defineStore } from 'pinia'
import { apiGet, apiPost } from '@/api/client'

export const useAuthStore = defineStore('auth', {
  state: () => ({
    currentUser: null, // { id, username, is_admin, created_at } | null
    checked: false, // 本次页面加载是否已经调用过 /api/me
    checking: false, // 防止并发重复调用
  }),
  getters: {
    isAuthenticated: (state) => state.currentUser !== null,
    isAdmin: (state) => state.currentUser?.is_admin === true,
  },
  actions: {
    async checkAuth() {
      if (this.checking) return
      this.checking = true
      try {
        this.currentUser = await apiGet('/api/me')
      } catch {
        this.currentUser = null
      } finally {
        this.checked = true
        this.checking = false
      }
    },
    async login(username, password) {
      this.currentUser = await apiPost('/api/login', { username, password })
      this.checked = true
      return this.currentUser
    },
    async logout() {
      try {
        await apiPost('/api/logout')
      } finally {
        this.currentUser = null
      }
    },
  },
})
