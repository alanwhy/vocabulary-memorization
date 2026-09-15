import { ref } from 'vue'
import { apiGet } from '@/api/client'
import { defaultReviewColors, normalizeReviewColors } from '@/utils/reviewLevel'

// 全局次数颜色配置：管理员在后台保存，登录用户在页面加载后读取同一份配置。
// 用模块级单例和在途 Promise 去重，避免单页多个单词组件同时请求同一个接口。
const reviewColors = ref(defaultReviewColors())
let loadingPromise = null

export function useReviewColors() {
  async function ensureReviewColors() {
    if (loadingPromise) return loadingPromise
    loadingPromise = apiGet('/api/review-colors')
      .then((data) => {
        reviewColors.value = normalizeReviewColors(data)
      })
      .catch(() => {
        // 配置读取失败时保持默认渐变，不能让一次网络失败影响单词展示。
      })
      .finally(() => {
        loadingPromise = null
      })
    return loadingPromise
  }

  return { reviewColors, ensureReviewColors }
}
