// 中文释义按「；」拆成义项的工具。后端 mergeSensesByPos 用全角分号把同词性的多条释义
// 连成一个 translation，前端要按义项标记「重要」就得再拆开，逐条渲染、逐条可点。
export function splitGlosses(translation) {
  if (!translation) return []
  return translation
    .split('；')
    .map((s) => s.trim())
    .filter(Boolean)
}
