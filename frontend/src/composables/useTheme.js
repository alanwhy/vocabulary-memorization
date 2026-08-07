import { ref } from 'vue'

// 主题由用户显式选择，不跟随系统偏好、也不按时间自动切换；选择结果存在 localStorage。
// theme 是模块级单例，任何组件调用 useTheme() 拿到的都是同一份状态。
const STORAGE_KEY = 'vocab_theme'

const theme = ref(localStorage.getItem(STORAGE_KEY) === 'dark' ? 'dark' : 'light')

// 深色变量统一挂在 html.dark 上（Element Plus 的 dark/css-vars.css 也是这个约定），
// 所以切换主题只要加/去掉这个 class。
function applyTheme() {
  document.documentElement.classList.toggle('dark', theme.value === 'dark')
}

// initTheme 在 app.mount 之前调用一次，让首屏就是正确的主题。
// index.html 里还有一段同样逻辑的内联脚本，负责在 JS bundle 加载前就避免闪白。
export function initTheme() {
  applyTheme()
}

export function useTheme() {
  function setTheme(next) {
    theme.value = next === 'dark' ? 'dark' : 'light'
    localStorage.setItem(STORAGE_KEY, theme.value)
    applyTheme()
  }

  function toggleTheme() {
    setTheme(theme.value === 'dark' ? 'light' : 'dark')
  }

  return { theme, setTheme, toggleTheme }
}
