// 时间格式化：原先 WordCard / ProfileView / AdminUsers / AdminDictionary 各抄了一份，统一收到这里。
export function formatTime(iso, fallback = '—') {
  if (!iso) return fallback
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return fallback
  const pad = (n) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}
