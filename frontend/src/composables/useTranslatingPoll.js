import { onUnmounted } from 'vue'

// 只要列表里还有查词中的单词，就定时刷新，直到后台把释义写回。
// SPA 里组件会被卸载（如从 / 跳到 /profile），必须清理定时器，否则轮询会在组件销毁后继续打接口。
export function useTranslatingPoll(words, refetch, intervalMs = 1500) {
  let timer = null

  function scheduleIfNeeded() {
    const hasPending = words.value.some((w) => w.translating)
    if (!hasPending) {
      stop()
      return
    }
    if (timer) return
    timer = setInterval(refetch, intervalMs)
  }

  function stop() {
    if (timer) {
      clearInterval(timer)
      timer = null
    }
  }

  onUnmounted(stop)

  return { scheduleIfNeeded, stop }
}
