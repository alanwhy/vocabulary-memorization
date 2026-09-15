// 次数颜色的纯计算工具。配置来自管理员设置；次数仍按每 2 次升一档，
// 因而改分段数只改变渐变细度，不会改变既有“次数越多颜色越靠近终止色”的语义。
const DEFAULT_REVIEW_COLORS = Object.freeze({
  start_color: '#e4f7e9',
  end_color: '#a11d1d',
  segments: 6,
})

export function defaultReviewColors() {
  return { ...DEFAULT_REVIEW_COLORS }
}

function isHexColor(value) {
  return typeof value === 'string' && /^#[0-9a-f]{6}$/i.test(value)
}

export function normalizeReviewColors(value) {
  const segments = Number(value?.segments)
  if (!isHexColor(value?.start_color) || !isHexColor(value?.end_color) || !Number.isInteger(segments) || segments < 2 || segments > 12) {
    return defaultReviewColors()
  }
  return {
    start_color: value.start_color.toLowerCase(),
    end_color: value.end_color.toLowerCase(),
    segments,
  }
}

export function countLevel(count, segments = DEFAULT_REVIEW_COLORS.segments) {
  const normalizedCount = Math.floor(Number(count))
  if (!Number.isFinite(normalizedCount) || normalizedCount < 1) return 0
  return Math.min(Math.floor((normalizedCount - 1) / 2) + 1, segments)
}

function hexToRgb(hex) {
  return [1, 3, 5].map((start) => Number.parseInt(hex.slice(start, start + 2), 16))
}

function rgbToHex([red, green, blue]) {
  return `#${[red, green, blue].map((part) => part.toString(16).padStart(2, '0')).join('')}`
}

function interpolatedColor(config, level) {
  const start = hexToRgb(config.start_color)
  const end = hexToRgb(config.end_color)
  const ratio = (level - 1) / (config.segments - 1)
  return rgbToHex(start.map((part, index) => Math.round(part + (end[index] - part) * ratio)))
}

function foregroundColor([red, green, blue]) {
  // YIQ 亮度足够时用深字，否则用白字，保证管理员选任意合法色仍可读。
  return red * 299 + green * 587 + blue * 114 >= 150000 ? '#1f2328' : '#ffffff'
}

// reviewColorStyle 同时服务次数徽标和例句命中词，确保相同次数永远使用相同颜色。
export function reviewColorStyle(count, colors) {
  const config = normalizeReviewColors(colors)
  const level = countLevel(count, config.segments)
  if (!level) return {}
  const background = interpolatedColor(config, level)
  return { backgroundColor: background, color: foregroundColor(hexToRgb(background)) }
}

export function countBadgeClass() {
  return ['count-badge']
}
