<script setup>
import { nextTick, onMounted, ref } from 'vue'
import { apiGet } from '@/api/client'
import { useWordStats } from '@/composables/useWordStats'
import { useEcharts, cssVar } from '@/composables/useEcharts'

const stats = ref({
  total_words: 0,
  total_reviews: 0,
  translating_count: 0,
  archived_words: 0,
  review_buckets: [],
  daily_additions: [],
  word_cloud: [],
  letter_stats: [],
})
const loaded = ref(false)
const errorMsg = ref('')

const { avgReviews, dailyTrend, reviewBuckets, wordCloudData, letterStatsData } = useWordStats(stats)

const trendRef = ref(null)
const distRef = ref(null)
const cloudRef = ref(null)
const letterRef = ref(null)

const trendChart = useEcharts(trendRef)
const distChart = useEcharts(distRef)
const cloudChart = useEcharts(cloudRef)
const letterChart = useEcharts(letterRef)

// 四张图统一配色：主色用界面 accent，坐标轴文字用 muted；数据都来自 stats 的后端聚合
function renderCharts() {
  const accent = cssVar('--accent', '#4f8cff')
  const muted = cssVar('--muted', '#9aa0a6')
  const splitLine = { lineStyle: { color: 'rgba(128,128,128,0.15)' } }
  const axisLine = { lineStyle: { color: muted } }

  trendChart.setOption({
    grid: { left: 40, right: 12, top: 24, bottom: 28 },
    tooltip: { trigger: 'axis' },
    xAxis: {
      type: 'category',
      data: dailyTrend.value.map((d) => d.label),
      axisLabel: { color: muted },
      axisLine,
    },
    yAxis: { type: 'value', minInterval: 1, axisLabel: { color: muted }, splitLine },
    series: [
      {
        type: 'bar',
        data: dailyTrend.value.map((d) => d.count),
        barMaxWidth: 22,
        itemStyle: { color: accent, borderRadius: [3, 3, 0, 0] },
      },
    ],
  })

  distChart.setOption({
    grid: { left: 72, right: 32, top: 12, bottom: 20 },
    tooltip: { trigger: 'axis' },
    xAxis: { type: 'value', minInterval: 1, axisLabel: { color: muted }, splitLine },
    yAxis: {
      type: 'category',
      data: reviewBuckets.value.map((b) => b.label),
      axisLabel: { color: muted, interval: 0 },
      axisLine,
    },
    series: [
      {
        type: 'bar',
        data: reviewBuckets.value.map((b) => b.count),
        barMaxWidth: 18,
        itemStyle: { color: accent, borderRadius: [0, 3, 3, 0] },
      },
    ],
  })

  cloudChart.setOption({
    tooltip: {
      formatter: (params) => {
        const d = params.data || {}
        return d.meaning ? `${d.name} ×${d.value}<br/>${d.meaning}` : `${d.name} ×${d.value}`
      },
    },
    series: [
      {
        type: 'wordCloud',
        shape: 'circle',
        sizeRange: [12, 44],
        rotationRange: [0, 0],
        gridSize: 6,
        textStyle: { color: accent, fontFamily: 'sans-serif' },
        data: wordCloudData.value,
      },
    ],
  })

  letterChart.setOption({
    grid: { left: 40, right: 12, top: 24, bottom: 28 },
    tooltip: { trigger: 'axis' },
    xAxis: {
      type: 'category',
      data: letterStatsData.value.map((d) => d.letter),
      axisLabel: { color: muted },
      axisLine,
    },
    yAxis: { type: 'value', minInterval: 1, axisLabel: { color: muted }, splitLine },
    series: [
      {
        type: 'bar',
        data: letterStatsData.value.map((d) => d.count),
        barMaxWidth: 22,
        itemStyle: { color: accent, borderRadius: [3, 3, 0, 0] },
      },
    ],
  })
}

onMounted(async () => {
  try {
    stats.value = await apiGet('/api/stats')
    loaded.value = true
    await nextTick()
    renderCharts()
  } catch {
    errorMsg.value = '统计数据加载失败，请刷新重试'
  }
})
</script>

<template>
  <div>
    <h1>统计</h1>

    <template v-if="stats.total_words">
      <div class="overview">
        <div class="card">
          <div class="num">{{ stats.total_words }}</div>
          <div class="label">总词汇量</div>
        </div>
        <div class="card">
          <div class="num">{{ stats.total_reviews }}</div>
          <div class="label">累计背诵次数</div>
        </div>
        <div class="card">
          <div class="num">{{ avgReviews }}</div>
          <div class="label">平均背诵次数</div>
        </div>
        <div class="card">
          <div class="num">{{ stats.archived_words }}</div>
          <div class="label">已归档单词数</div>
        </div>
      </div>

      <h2>最近 14 天新增趋势</h2>
      <div class="card"><div ref="trendRef" class="chart"></div></div>

      <h2>背诵次数分布</h2>
      <div class="card"><div ref="distRef" class="chart chart-dist"></div></div>

      <h2>近 7 天词云</h2>
      <div class="card"><div ref="cloudRef" class="chart chart-cloud"></div></div>

      <h2>开头字母统计</h2>
      <div class="card"><div ref="letterRef" class="chart"></div></div>
    </template>

    <div class="empty" v-else-if="errorMsg">{{ errorMsg }}</div>
    <div class="empty" v-else-if="loaded">还没有单词记录，先去背几个单词吧</div>
  </div>
</template>

<style scoped>
.overview {
  display: grid;
  /* minmax(0, 1fr)：数字很长时 1fr 会被内容撑宽，四张卡就不等宽了 */
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 10px;
  align-items: stretch;
}
.overview .card {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  min-height: 84px;
  padding: 16px 8px;
  text-align: center;
}
/* main.css 的 .card + .card { margin-top: 16px } 会命中网格里第 2/3/4 张卡，
   把它们整体往下推 16px——这就是头部四张卡看起来高低不齐的原因 */
.overview .card + .card {
  margin-top: 0;
}
.overview .num {
  font-size: 22px;
  font-weight: 700;
  color: var(--accent);
}
.overview .label {
  font-size: 12px;
  color: var(--muted);
  margin-top: 4px;
}
.chart {
  width: 100%;
  height: 220px;
}
.chart-dist {
  height: 220px;
}
.chart-cloud {
  height: 320px;
}
@media (max-width: 480px) {
  .overview {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>
