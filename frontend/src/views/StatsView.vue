<script setup>
import { onMounted, ref } from 'vue'
import { apiGet } from '@/api/client'
import { useWordStats } from '@/composables/useWordStats'

const words = ref([])

const {
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
} = useWordStats(words)

onMounted(async () => {
  words.value = await apiGet('/api/words')
})
</script>

<template>
  <div>
    <h1>统计</h1>

    <template v-if="words.length">
      <div class="overview">
        <div class="card">
          <div class="num">{{ words.length }}</div>
          <div class="label">总词汇量</div>
        </div>
        <div class="card">
          <div class="num">{{ totalReviews }}</div>
          <div class="label">累计背诵次数</div>
        </div>
        <div class="card">
          <div class="num">{{ avgReviews }}</div>
          <div class="label">平均背诵次数</div>
        </div>
        <div class="card">
          <div class="num">{{ translatingCount }}</div>
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

      <h2>最久未复习</h2>
      <ul class="stale-list">
        <li class="stale-item" v-for="w in staleWords" :key="w.id">
          <span class="word">{{ w.display_word }}</span>
          <span class="days">{{ daysSince(w.last_reviewed_at) }} 天未复习</span>
        </li>
      </ul>
    </template>

    <div class="empty" v-else>还没有单词记录，先去背几个单词吧</div>
  </div>
</template>

<style scoped>
.overview {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 10px;
}
.overview .card {
  text-align: center;
  padding: 16px 8px;
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
ul.stale-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.stale-item {
  background: var(--card-bg);
  border: 1px solid var(--border);
  border-radius: 10px;
  padding: 10px 14px;
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.stale-item .word {
  font-size: 15px;
  font-weight: 600;
}
.stale-item .days {
  font-size: 12px;
  color: var(--muted);
}
</style>
