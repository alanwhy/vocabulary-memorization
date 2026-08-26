// 统一请求封装：所有 /api 调用都走这里，替代旧页面里到处手写的 fetch + 错误处理。
// 鉴权改为 Bearer token：token 存 localStorage，每次请求从这里读并拼 Authorization 头，
// 不再依赖 cookie（也就不再需要 credentials: 'include'）。
export const TOKEN_KEY = 'vocab_token'

export function getToken() {
  return localStorage.getItem(TOKEN_KEY)
}

async function request(url, opts = {}) {
  const token = getToken()
  const headers = { ...opts.headers }
  if (token) {
    headers.Authorization = `Bearer ${token}`
  }

  const res = await fetch(url, { ...opts, headers })

  if (res.status === 401 && url !== '/api/me') {
    // 会话过期：清空登录态和 token 并跳回首页登录框，但不要对 /api/me 自身的 401 做这个处理，
    // 否则未登录时访问 / 触发的 checkAuth() 会导致跳转死循环。
    localStorage.removeItem(TOKEN_KEY)
    const { useAuthStore } = await import('@/stores/auth')
    const auth = useAuthStore()
    auth.currentUser = null
    const router = (await import('@/router')).default
    if (router.currentRoute.value.path !== '/') router.push('/')
  }

  const data = await res.json().catch(() => ({}))
  if (!res.ok) {
    throw new Error(data.error || '请求失败')
  }
  return data
}

export const apiGet = (url) => request(url)

export const apiPost = (url, body) =>
  request(url, {
    method: 'POST',
    headers: body ? { 'Content-Type': 'application/json' } : {},
    body: body ? JSON.stringify(body) : undefined,
  })

export const apiPut = (url, body) =>
  request(url, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })

export const apiDelete = (url) => request(url, { method: 'DELETE' })
