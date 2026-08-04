// 统一请求封装：所有 /api 调用都走这里，替代旧页面里到处手写的 fetch + 错误处理。
// credentials: 'include' 在 dev（Vite proxy 转发）和生产（同源）下都需要，保持一份逻辑两边通用。
async function request(url, opts = {}) {
  const res = await fetch(url, { credentials: 'include', ...opts })

  if (res.status === 401 && url !== '/api/me') {
    // 会话过期：清空登录态并跳回首页登录框，但不要对 /api/me 自身的 401 做这个处理，
    // 否则未登录时访问 / 触发的 checkAuth() 会导致跳转死循环。
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
