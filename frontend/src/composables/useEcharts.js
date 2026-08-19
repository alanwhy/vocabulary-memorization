import * as echarts from 'echarts'
import 'echarts-wordcloud'
import { onBeforeUnmount } from 'vue'

// 从 CSS 变量读取颜色，让图表配色跟随界面主题（--accent / --muted / --text）
export function cssVar(name, fallback = '') {
  const v = getComputedStyle(document.documentElement).getPropertyValue(name).trim()
  return v || fallback
}

// echarts 图表生命周期封装：容器可能因 v-if 延迟渲染，所以 setOption 首次调用时才真正 init；
// 统一处理窗口 resize 与组件卸载时的 dispose，避免每个图表各写一遍样板代码。
export function useEcharts(elRef) {
  let chart = null

  function ensure() {
    if (chart) return chart
    const el = elRef.value
    if (!el) return null
    chart = echarts.init(el)
    return chart
  }

  const onResize = () => {
    if (chart) chart.resize()
  }
  window.addEventListener('resize', onResize)

  onBeforeUnmount(() => {
    window.removeEventListener('resize', onResize)
    if (chart) {
      chart.dispose()
      chart = null
    }
  })

  // notMerge 传 true：数据变化时整体替换 option，避免残留上一次的旧图元
  function setOption(option) {
    const c = ensure()
    if (c) c.setOption(option, true)
  }

  return { setOption }
}
