// 把一句英文例句按「单词 / 非单词」切分，逐词返回词库中的出现次数。
// 例句命中和次数徽标共用颜色计算，因而这里保留 count 而不是提前固化颜色档位。
export function tokenizeExample(text, lookup) {
  return text
    .split(/([A-Za-z']+)/)
    .filter((s) => s !== '')
    .map((part) => {
      if (/^[A-Za-z']+$/.test(part)) {
        const key = part.toLowerCase()
        return { text: part, count: lookup(key), isWord: true }
      }
      return { text: part, count: 0, isWord: false }
    })
}

// 把 "chance（机会）" 这类「英文词 + 中文释义」拆成 { word, rest }：
// word 是可点击查询的英文词，rest 是紧随的中文释义。不再做词库高亮，只负责拆分。
export function splitWordRef(ref) {
  const m = ref.match(/^([A-Za-z'-]+)(.*)$/)
  if (!m) return { word: ref, rest: '' }
  return { word: m[1], rest: m[2] }
}
