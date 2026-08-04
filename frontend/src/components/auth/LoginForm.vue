<script setup>
import { reactive, ref } from 'vue'
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const loginForm = reactive({ username: '', password: '' })
const loginError = ref('')
const loggingIn = ref(false)

async function login() {
  if (loggingIn.value) return
  if (!loginForm.username || !loginForm.password) {
    loginError.value = '请输入用户名和密码'
    return
  }
  loggingIn.value = true
  loginError.value = ''
  try {
    await auth.login(loginForm.username, loginForm.password)
    loginForm.password = ''
  } catch (e) {
    loginError.value = e.message || '登录失败'
  } finally {
    loggingIn.value = false
  }
}
</script>

<template>
  <div class="login-box">
    <h1>背单词</h1>
    <input
      type="text"
      v-model="loginForm.username"
      placeholder="用户名"
      autofocus
      @keyup.enter="login"
    />
    <input
      type="password"
      v-model="loginForm.password"
      placeholder="密码"
      @keyup.enter="login"
    />
    <p class="login-error">{{ loginError }}</p>
    <button :disabled="loggingIn" @click="login">登录</button>
    <p class="login-contact">忘记密码或需要开通账号，请联系 alanwhy0528@gmail.com</p>
  </div>
</template>

<style scoped>
.login-box {
  max-width: 360px;
  margin: 80px auto 0;
  background: var(--card-bg);
  border: 1px solid var(--border);
  border-radius: 12px;
  padding: 28px;
}
.login-box h1 {
  margin-bottom: 24px;
}
.login-box input[type='text'],
.login-box input[type='password'] {
  width: 100%;
  padding: 12px 14px;
  font-size: 15px;
  border: 1px solid var(--border);
  border-radius: 8px;
  background: var(--bg);
  color: var(--text);
  outline: none;
  margin-bottom: 12px;
}
.login-box button {
  width: 100%;
  padding: 12px;
  font-size: 15px;
  border: none;
  border-radius: 8px;
  background: var(--accent);
  color: #fff;
  cursor: pointer;
}
.login-box button:disabled {
  opacity: 0.6;
  cursor: default;
}
.login-error {
  color: var(--danger);
  font-size: 13px;
  margin: -4px 0 12px;
  min-height: 16px;
}
.login-contact {
  color: var(--muted);
  font-size: 12px;
  margin-top: 12px;
  text-align: center;
}
</style>
