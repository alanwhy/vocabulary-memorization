import { onUnmounted, watch } from 'vue'

// 滚动到列表底部的哨兵元素时触发加载下一页。
// 页面是整页（window）滚动，Element Plus 的 v-infinite-scroll 需要一个固定高度的滚动容器，
// 这里用 IntersectionObserver 更合适：不用给列表写死高度，也不用监听 scroll 事件做节流。
export function useInfiniteScroll(sentinelRef, onHit, { rootMargin = '200px' } = {}) {
  let observer = null

  function disconnect() {
    if (observer) {
      observer.disconnect()
      observer = null
    }
  }

  // 哨兵元素挂在 v-if 里，会随列表状态反复挂载/卸载，所以要 watch ref 重新观察
  watch(
    sentinelRef,
    (el) => {
      disconnect()
      if (!el) return
      observer = new IntersectionObserver(
        (entries) => {
          if (entries.some((e) => e.isIntersecting)) onHit()
        },
        { rootMargin },
      )
      observer.observe(el)
    },
    { immediate: true, flush: 'post' },
  )

  onUnmounted(disconnect)

  return { disconnect }
}
