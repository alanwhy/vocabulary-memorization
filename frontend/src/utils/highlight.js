// 三个例句入口共用此工具：词形只用于查找次数，原文仍用于展示和点击查词。
// 不规则变化用显式表约束，避免把 go + ed 等拼写误认为有效词形。
const irregularForms = new Map(Object.entries({
  be: ['am', 'is', 'are', 'was', 'were', 'been', 'being'],
  have: ['has', 'had', 'having'],
  do: ['does', 'did', 'done', 'doing'],
  go: ['goes', 'went', 'gone', 'going'],
  run: ['ran'],
  sing: ['sang', 'sung'],
  swim: ['swam', 'swum'],
  ring: ['rang', 'rung', 'ringed'],
  make: ['made'],
  take: ['took', 'taken'],
  give: ['gave', 'given'],
  get: ['got', 'gotten'],
  come: ['came'],
  become: ['became'],
  see: ['saw', 'seen'],
  say: ['said'],
  tell: ['told'],
  think: ['thought'],
  know: ['knew', 'known'],
  find: ['found'],
  buy: ['bought'],
  bring: ['brought'],
  catch: ['caught'],
  teach: ['taught'],
  feel: ['felt'],
  keep: ['kept'],
  leave: ['left'],
  lose: ['lost'],
  meet: ['met'],
  pay: ['paid'],
  read: ['read'],
  sell: ['sold'],
  send: ['sent'],
  sit: ['sat'],
  stand: ['stood'],
  understand: ['understood'],
  write: ['wrote', 'written'],
  eat: ['ate', 'eaten'],
  drink: ['drank', 'drunk'],
  drive: ['drove', 'driven'],
  speak: ['spoke', 'spoken'],
  break: ['broke', 'broken'],
  choose: ['chose', 'chosen'],
  fall: ['fell', 'fallen'],
  fly: ['flew', 'flown'],
  forget: ['forgot', 'forgotten'],
  grow: ['grew', 'grown'],
  hold: ['held'],
  wear: ['wore', 'worn'],
  win: ['won'],
  cut: ['cut'],
  hit: ['hit'],
  put: ['put'],
  set: ['set'],
  let: ['let'],
  shut: ['shut'],
  cost: ['cost'],
  hurt: ['hurt'],
}))
const irregularRoots = new Map()
for (const [root, forms] of irregularForms) {
  for (const form of forms) {
    const roots = irregularRoots.get(form) || []
    irregularRoots.set(form, [...roots, root])
  }
}

// 正向校验候选：短 CVC 词需双写（hop→hopping），不能让 hoping 错配到 hop。
// 长词重音无法仅凭拼写判定，保留普通/双写两种候选以覆盖 opened、preferred 等。
function wordForms(key) {
  const forms = new Set([key, ...(irregularForms.get(key) || [])])
  if (!/^[a-z]{3,}$/.test(key) || key === 'have') return forms
  const consonantY = /[^aeiou]y$/.test(key)
  forms.add(consonantY ? `${key.slice(0, -1)}ies` : `${key}${/(s|x|z|ch|sh)$/.test(key) ? 'es' : 's'}`)
  const cvc = /^[^aeiou]*[aeiou][^aeiouwxy]$/.test(key)
  const doubled = `${key}${key.at(-1)}`
  const stem = /ic$/.test(key) ? `${key}k` : cvc ? doubled : key
  if (!irregularForms.has(key)) {
    const past = consonantY
      ? `${key.slice(0, -1)}ied`
      : key.endsWith('e') ? `${key}d` : `${stem}ed`
    forms.add(past)
  }
  if (/ie$/.test(key)) forms.add(`${key.slice(0, -2)}ying`)
  // singe 保留 e，避免把 singing（sing 的词形）误认成 singe。
  else if (key === 'singe') forms.add(`${key}ing`)
  else if (/[^eoy]e$/.test(key)) forms.add(`${key.slice(0, -1)}ing`)
  else forms.add(`${stem}ing`)
  if (!cvc && !/ic$/.test(key) && /[^aeiou][aeiou][^aeiouwxy]$/.test(key)) {
    if (!irregularForms.has(key)) forms.add(`${doubled}ed`)
    forms.add(`${doubled}ing`)
  }
  return forms
}

// 只还原有限词尾，再用正向规则与词库双重确认；不扫描词库、不使用前缀匹配。
function wordRoots(key) {
  const roots = new Set(irregularRoots.get(key) || [])
  const addStem = (stem) => {
    roots.add(stem)
    roots.add(`${stem}e`)
    if (/([a-z])\1$/.test(stem)) roots.add(stem.slice(0, -1))
    if (stem.endsWith('ick')) roots.add(stem.slice(0, -1))
  }
  if (key.endsWith('s')) roots.add(key.slice(0, -1))
  if (key.endsWith('es')) roots.add(key.slice(0, -2))
  if (/i(es|ed)$/.test(key)) roots.add(`${key.slice(0, -3)}y`)
  if (key.endsWith('ed')) addStem(key.slice(0, -2))
  if (key.endsWith('ing')) addStem(key.slice(0, -3))
  if (key.endsWith('ying')) roots.add(`${key.slice(0, -4)}ie`)
  return [...roots].filter((root) => wordForms(root).has(key))
}

// 当前词的有效词形优先沿用当前词次数；其它 token 先精确查找，再尝试词形还原。
// lookup 可由闪卡限定为只接受当前词，因此共享规则不会扩大闪卡高亮范围。
export function tokenizeExample(text, lookup, preferredWord = '') {
  const preferredKey = preferredWord.toLowerCase()
  const preferredForms = wordForms(preferredKey)
  return text
    .split(/([A-Za-z']+)/)
    .filter((s) => s !== '')
    .map((part) => {
      if (/^[A-Za-z']+$/.test(part)) {
        const key = part.toLowerCase()
        let count = preferredForms.has(key) ? lookup(preferredKey) : 0
        count ||= lookup(key)
        if (!count) {
          for (const root of wordRoots(key)) {
            count = lookup(root)
            if (count) break
          }
        }
        return { text: part, count, isWord: true }
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
