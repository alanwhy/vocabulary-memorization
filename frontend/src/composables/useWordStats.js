import { computed } from 'vue'

const TREND_DAYS = 14

const BUCKET_LABELS = ['1 次', '2-3 次', '4-6 次', '7 次以上']

// 把 /api/stats 的聚合结果整理成图表要用的形状。
// 数值本身由后端 SQL 算出——单词列表分页后前端手里没有全量数据，算不出这些统计。
export function useWordStats(stats) {
  const avgReviews = computed(() => {
    const total = stats.value.total_words || 0
    if (!total) return '0'
    return ((stats.value.total_reviews || 0) / total).toFixed(1)
  })

  function dateKey(d) {
    const pad = (n) => String(n).padStart(2, '0')
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
  }

  // 后端只返回有新增的那些天，这里补齐成连续 14 天，柱状图才不会缺格
  const dailyTrend = computed(() => {
    const counts = {}
    for (const item of stats.value.daily_additions || []) {
      counts[item.date] = item.count
    }
    const days = []
    const today = new Date()
    for (let i = TREND_DAYS - 1; i >= 0; i--) {
      const d = new Date(today)
      d.setDate(d.getDate() - i)
      const key = dateKey(d)
      const pad = (n) => String(n).padStart(2, '0')
      days.push({
        date: key,
        label: `${pad(d.getMonth() + 1)}-${pad(d.getDate())}`,
        count: counts[key] || 0,
      })
    }
    return days
  })

  const reviewBuckets = computed(() =>
    BUCKET_LABELS.map((label, idx) => ({ label, count: stats.value.review_buckets?.[idx] || 0 })),
  )

  // 词云数据：echarts-wordcloud 需要 { name, value } 形状
  const wordCloudData = computed(() =>
    (stats.value.word_cloud || []).map((it) => ({ name: it.word, value: it.count })),
  )

  // 开头字母统计：后端已按字母排序，直接给 echarts 柱状图用
  const letterStatsData = computed(() => stats.value.letter_stats || [])

  return { avgReviews, dailyTrend, reviewBuckets, wordCloudData, letterStatsData }
}
