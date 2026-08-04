import { computed } from 'vue'

// 从单词列表派生统计视图所需的一切数值，供 StatsView 使用。
export function useWordStats(words) {
  const totalReviews = computed(() => words.value.reduce((sum, w) => sum + w.review_count, 0))

  const avgReviews = computed(() => {
    if (!words.value.length) return '0'
    return (totalReviews.value / words.value.length).toFixed(1)
  })

  const translatingCount = computed(() => words.value.filter((w) => w.translating).length)

  function dateKey(d) {
    const pad = (n) => String(n).padStart(2, '0')
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
  }

  const dailyTrend = computed(() => {
    const counts = {}
    for (const w of words.value) {
      const key = dateKey(new Date(w.first_added_at))
      counts[key] = (counts[key] || 0) + 1
    }
    const days = []
    const today = new Date()
    for (let i = 13; i >= 0; i--) {
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

  const maxDailyCount = computed(() => Math.max(1, ...dailyTrend.value.map((d) => d.count)))

  const reviewBuckets = computed(() => {
    const buckets = [
      { label: '1 次', min: 1, max: 1, count: 0 },
      { label: '2-3 次', min: 2, max: 3, count: 0 },
      { label: '4-6 次', min: 4, max: 6, count: 0 },
      { label: '7 次以上', min: 7, max: Infinity, count: 0 },
    ]
    for (const w of words.value) {
      const b = buckets.find((b) => w.review_count >= b.min && w.review_count <= b.max)
      if (b) b.count++
    }
    return buckets
  })

  const maxBucketCount = computed(() => Math.max(1, ...reviewBuckets.value.map((b) => b.count)))

  const staleWords = computed(() =>
    [...words.value].sort((a, b) => new Date(a.last_reviewed_at) - new Date(b.last_reviewed_at)).slice(0, 5),
  )

  function barHeight(count, max) {
    const pct = max > 0 ? (count / max) * 100 : 0
    return `${Math.max(pct, count > 0 ? 4 : 0)}%`
  }

  function daysSince(iso) {
    const diffMs = Date.now() - new Date(iso).getTime()
    return Math.max(0, Math.floor(diffMs / 86400000))
  }

  return {
    totalReviews,
    avgReviews,
    translatingCount,
    dailyTrend,
    maxDailyCount,
    reviewBuckets,
    maxBucketCount,
    staleWords,
    barHeight,
    daysSince,
  }
}
