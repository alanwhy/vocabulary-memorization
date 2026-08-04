export function formatTime(iso) {
  const d = new Date(iso);
  const pad = (n) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

export async function fetchJSON(url, opts) {
  const res = await fetch(url, opts);
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    throw new Error(data.error || "请求失败");
  }
  return data;
}

// requireLogin 检查登录态，未登录直接跳回首页；用于"必须登录才能看"的页面（统计、后台、归档）
export async function requireLogin() {
  try {
    const res = await fetch("/api/me");
    if (!res.ok) {
      window.location.href = "/";
      return null;
    }
    return await res.json();
  } catch (e) {
    window.location.href = "/";
    return null;
  }
}
