<script setup>
import { onMounted, ref } from 'vue'
import { apiGet } from '@/api/client'
import { useWordStats } from '@/composables/useWordStats'

const stats = ref({
  total_words: 0,
  total_reviews: 0,
  translating_count: 0,
  review_buckets: [],
  daily_additions: [],
})
const loaded = ref(false)
const errorMsg = ref('')

const { avgReviews, dailyTrend, maxDailyCount, reviewBuckets, maxBucketCount, barHeight } =
  useWordStats(stats)

onMounted(async () => {
  try {
    stats.value = await apiGet('/api/stats')
    loaded.value = true
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
          <div class="num">{{ stats.translating_count }}</div>
          <div class="label">查词中</div>
        </div>
      </div>

      <h2>最近 14 天新增趋势</h2>
      <div class="card">
        <div class="bar-chart">
          <div class="bar-col" v-for="d in dailyTrend" :key="d.date">
            <span class="bar-value">{{ d.count || '' }}</span>
            <div class="bar" :style="{ height: barHeight(d.count, maxDailyCount) }"></div>
            <span class="bar-label">{{ d.label }}</span>
          </div>
        </div>
      </div>

      <h2>背诵次数分布</h2>
      <div class="card dist-chart">
        <div class="dist-row" v-for="b in reviewBuckets" :key="b.label">
          <span class="dist-label">{{ b.label }}</span>
          <div class="dist-bar-wrap">
            <div class="dist-bar" :style="{ width: barHeight(b.count, maxBucketCount) }"></div>
          </div>
          <span class="dist-count">{{ b.count }}</span>
        </div>
      </div>

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
.bar-chart {
  display: flex;
  align-items: flex-end;
  gap: 4px;
  height: 120px;
}
.bar-col {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  height: 100%;
  justify-content: flex-end;
  gap: 4px;
  min-width: 0;
}
.bar-col .bar-value {
  font-size: 11px;
  color: var(--muted);
}
.bar-col .bar {
  width: 100%;
  max-width: 22px;
  background: var(--accent);
  border-radius: 3px 3px 0 0;
  min-height: 2px;
}
.bar-col .bar-label {
  font-size: 10px;
  color: var(--muted);
  white-space: nowrap;
  transform: rotate(-40deg);
  transform-origin: top center;
  margin-top: 6px;
}
.dist-chart {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.dist-row {
  display: flex;
  align-items: center;
  gap: 10px;
}
.dist-row .dist-label {
  font-size: 13px;
  color: var(--muted);
  width: 72px;
  flex-shrink: 0;
}
.dist-row .dist-bar-wrap {
  flex: 1;
  background: var(--accent-soft);
  border-radius: 6px;
  overflow: hidden;
  height: 18px;
}
.dist-row .dist-bar {
  height: 100%;
  background: var(--accent);
  border-radius: 6px;
  min-width: 2px;
}
.dist-row .dist-count {
  font-size: 13px;
  font-weight: 600;
  width: 32px;
  text-align: right;
  flex-shrink: 0;
}
@media (max-width: 480px) {
  .overview {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>
