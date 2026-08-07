// 出现次数 -> 配色档位（1~6）。档位边界按 1/3/5/7/9/11 划分，
// 颜色从浅绿经黄、橙走到红、深红，次数越多越「烫」，扫视时一眼能挑出背得最多的词。
// 注意这套档位和统计页「背诵次数分布」的分桶（1 / 2-3 / 4-6 / 7 次以上）是两套口径：
// 那里是为了把词汇量分成几组看分布，这里是为了让单个数字有冷热感，不必强行对齐。
// 对应颜色在 assets/main.css 里以 --count-N-bg/-fg 定义，class 为 .count-badge--lN。
export function countLevel(count) {
  if (count >= 11) return 6
  if (count >= 9) return 5
  if (count >= 7) return 4
  if (count >= 5) return 3
  if (count >= 3) return 2
  return 1
}

// countBadgeClass 直接给出徽标要用的 class 列表
export function countBadgeClass(count) {
  return ['count-badge', `count-badge--l${countLevel(count)}`]
}
