import { defineStore } from 'pinia'
import { apiGet, apiPost, TOKEN_KEY } from '@/api/client'

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
        // 没有 token 就没必要请求 /api/me，直接当作未登录
        if (!localStorage.getItem(TOKEN_KEY)) {
          this.currentUser = null
          return
        }
        this.currentUser = await apiGet('/api/me')
      } catch {
        this.currentUser = null
      } finally {
        this.checked = true
        this.checking = false
      }
    },
    async login(username, password) {
      const { token, user } = await apiPost('/api/login', { username, password })
      localStorage.setItem(TOKEN_KEY, token)
      this.currentUser = user
      this.checked = true
      return user
    },
    async logout() {
      try {
        await apiPost('/api/logout')
      } finally {
        localStorage.removeItem(TOKEN_KEY)
        this.currentUser = null
      }
    },
  },
})
