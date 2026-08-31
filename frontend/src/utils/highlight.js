import { countLevel } from './reviewLevel'

// 把一句英文例句按「单词 / 非单词」切分，逐词返回 { text, level, isWord }。
// 不再按全局词库命中高亮：只高亮「当前单词」本身（currentWordKey 为当前卡片单词的小写 key），
// 其余词一律 level 0。lookup 只用来取当前单词的出现次数档位，其它词不再查词库着色。
export function tokenizeExample(text, lookup, currentWordKey) {
  return text
    .split(/([A-Za-z']+)/)
    .filter((s) => s !== '')
    .map((part) => {
      if (/^[A-Za-z']+$/.test(part)) {
        const key = part.toLowerCase()
        const count = key === currentWordKey ? lookup(key) : 0
        return { text: part, level: count ? countLevel(count) : 0, isWord: true }
      }
      return { text: part, level: 0, isWord: false }
    })
}

// 把 "chance（机会）" 这类「英文词 + 中文释义」拆成 { word, rest }：
// word 是可点击查询的英文词，rest 是紧随的中文释义。不再做词库高亮，只负责拆分。
export function splitWordRef(ref) {
  const m = ref.match(/^([A-Za-z'-]+)(.*)$/)
  if (!m) return { word: ref, rest: '' }
  return { word: m[1], rest: m[2] }
}
