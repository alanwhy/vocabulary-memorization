import { countLevel } from './reviewLevel'

// 把一句英文例句按「单词 / 非单词」切分，逐词查词库，标注命中档位。
// lookup 是 useVocabularyIndex 提供的查询函数；返回 [{ text, level, isWord }]，
// level 为 0 表示该词不在词库，>0 表示命中词库且按出现次数分的档位（1~6）。
export function tokenizeExample(text, lookup) {
  return text
    .split(/([A-Za-z']+)/)
    .filter((s) => s !== '')
    .map((part) => {
      if (/^[A-Za-z']+$/.test(part)) {
        const count = lookup(part.toLowerCase())
        return { text: part, level: count ? countLevel(count) : 0, isWord: true }
      }
      return { text: part, level: 0, isWord: false }
    })
}

// 把 "chance（机会）" 这类「英文词 + 中文释义」拆成 { word, rest, level }：
// word 是英文部分（用于查词库高亮），rest 是紧随的中文释义，level 为命中档位（0=不命中）。
export function splitWordRef(ref, lookup) {
  const m = ref.match(/^([A-Za-z'-]+)(.*)$/)
  if (!m) return { word: ref, rest: '', level: 0 }
  const count = lookup(m[1].toLowerCase())
  return { word: m[1], rest: m[2], level: count ? countLevel(count) : 0 }
}
