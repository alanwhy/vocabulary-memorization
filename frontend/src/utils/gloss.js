// 中文释义按「；」拆成义项的工具。后端 mergeSensesByPos 用全角分号把同词性的多条释义
// 连成一个 translation；同时兼容半角分号，分隔符不参与义项标记或高亮。
export function splitGlosses(translation) {
  if (!translation) return []
  return translation
    .split(/[；;]/)
    .map((s) => s.trim())
    .filter(Boolean)
}
