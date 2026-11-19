import test from 'node:test'
import assert from 'node:assert/strict'
import { registerHooks } from 'node:module'

// 只解析生产代码已有别名，保留真实 Vue 响应式状态、请求封装和队列动作。
registerHooks({
  resolve(specifier, context, nextResolve) {
    if (specifier === '@/api/client') {
      return { url: new URL('../api/client.js', import.meta.url).href, shortCircuit: true }
    }
    return nextResolve(specifier, context)
  },
})

let fixtureID = 0

/** 创建隔离的模块单例与受控 HTTP 响应，记录实际请求以验证重练不会重新拉队列。 */
async function fixture(t, ids = [1, 2, 3]) {
  const originalStorage = Object.getOwnPropertyDescriptor(globalThis, 'localStorage')
  Object.defineProperty(globalThis, 'localStorage', {
    configurable: true,
    value: { getItem: () => null },
  })
  t.after(() => {
    if (originalStorage) Object.defineProperty(globalThis, 'localStorage', originalStorage)
    else delete globalThis.localStorage
  })
  const requests = []
  const transport = {
    words: ids.map((id) => ({ id, word_key: `word${id}`, senses: [] })),
    review: async () => new Response('{}'),
    queue: async () => Response.json(transport.words),
  }
  t.mock.method(globalThis, 'fetch', async (url, options) => {
    requests.push({ url, body: options?.body ? JSON.parse(options.body) : undefined })
    if (url === '/api/flashcards/queue') return transport.queue()
    if (url === '/api/flashcards/review') return transport.review()
    throw new Error(`未预期请求：${url}`)
  })
  const { useFlashcardQueue } = await import(`./useFlashcardQueue.js?test=${fixtureID++}`)
  const state = useFlashcardQueue()
  await state.loadQueue()
  return { state, transport, requests, useFlashcardQueue }
}

test('重练只保留本轮不认识和模糊，连续重练剔除已记住的词', async (t) => {
  const { state: s, requests } = await fixture(t, [1, 2, 3, 4])
  for (const rating of ['good', 'again', 'fuzzy', 'again']) await s.rate(rating)
  assert.equal(s.finished.value, true)
  assert.equal(s.retryCount.value, 3)
  s.restartGroup()
  assert.deepEqual(s.cards.value.map((word) => word.id), [2, 3, 4])
  assert.equal(s.index.value, 0)
  assert.equal(s.flipped.value, false)
  assert.deepEqual(s.groupResults.value, [])
  assert.equal(s.doneCount.value, 4)
  assert.equal(requests.filter((request) => request.url.endsWith('/queue')).length, 1)

  // 重复点击不会抹掉已进入的新一轮状态。
  s.restartGroup()
  assert.deepEqual(s.cards.value.map((word) => word.id), [2, 3, 4])
  for (const rating of ['good', 'fuzzy', 'good']) await s.rate(rating)
  assert.deepEqual(s.groupCounts.value, { again: 0, fuzzy: 1, good: 2 })
  s.restartGroup()
  assert.deepEqual(s.cards.value.map((word) => word.id), [3])
  await s.rate('good')
  assert.equal(s.retryCount.value, 0)
  s.restartGroup()
  assert.equal(s.finished.value, true)
  assert.equal(s.doneCount.value, 8)
  assert.deepEqual(requests.filter((request) => request.body).map((request) => request.body.id), [1, 2, 3, 4, 2, 3, 4, 3])
})

test('未完成时不能重练，切换调用方保留重练进度和内容更新能力', async (t) => {
  const { state: s, useFlashcardQueue } = await fixture(t)
  await s.rate('again')
  s.restartGroup()
  assert.equal(s.current.value.id, 2)
  assert.equal(s.groupResults.value.length, 1)
  await s.rate('good')
  await s.rate('fuzzy')
  s.restartGroup()
  await s.rate('good')
  const resumed = useFlashcardQueue()
  resumed.ensureQueue()
  assert.equal(resumed.current.value.id, 3)
  assert.equal(resumed.doneCount.value, 4)
  assert.equal(resumed.groupResults.value.length, 1)
  resumed.updateContent({ id: 3, senses: [{ translation: '更新释义' }], translating: false })
  assert.equal(s.current.value.senses[0].translation, '更新释义')
})

test('提交中与失败评分均不会提前重练或推进，重试成功后才能计入结果', async (t) => {
  const { state: s, transport } = await fixture(t, [1])
  let complete
  transport.review = () => new Promise((resolve) => { complete = resolve })
  const pending = s.rate('again')
  assert.equal(s.submitting.value, true)
  s.restartGroup()
  assert.equal(s.current.value.id, 1)
  complete(Response.json({ error: '评分失败' }, { status: 500 }))
  await pending
  assert.equal(s.error.value, '评分失败')
  assert.equal(s.index.value, 0)
  assert.equal(s.doneCount.value, 0)
  assert.deepEqual(s.groupResults.value, [])
  transport.review = async () => new Response('{}')
  await s.rate('fuzzy')
  s.restartGroup()
  assert.equal(s.current.value.id, 1)
  assert.equal(s.error.value, '')
})

test('下一组加载期间不能重练，加载失败后旧结果仍可重练', async (t) => {
  const { state: s, transport } = await fixture(t, [1, 2])
  await s.rate('again')
  await s.rate('good')
  let complete
  transport.queue = () => new Promise((resolve) => { complete = resolve })
  const pending = s.loadQueue()
  s.restartGroup()
  assert.equal(s.index.value, 2)
  assert.equal(s.groupResults.value.length, 2)
  complete(Response.json({ error: '加载失败' }, { status: 500 }))
  await pending
  assert.equal(s.finished.value, true)
  assert.equal(s.error.value, '加载失败')
  s.restartGroup()
  assert.deepEqual(s.cards.value.map((word) => word.id), [1])
  assert.equal(s.error.value, '')
})

test('空队列和全认识无重练；加载下一组保持累计次数并清空上轮结果', async (t) => {
  const { state: s, transport } = await fixture(t, [])
  assert.equal(s.allDone.value, true)
  s.restartGroup()
  assert.deepEqual(s.cards.value, [])
  transport.words = [{ id: 5 }]
  await s.loadQueue()
  await s.rate('good')
  s.restartGroup()
  assert.equal(s.finished.value, true)
  assert.equal(s.groupResults.value.length, 1)
  transport.words = [{ id: 6 }]
  await s.loadQueue()
  assert.equal(s.current.value.id, 6)
  assert.equal(s.doneCount.value, 1)
  assert.deepEqual(s.groupResults.value, [])
})

test('历史 hard 评分不加入当前组重练', async (t) => {
  const { state: s } = await fixture(t, [1, 2])
  await s.rate('hard')
  await s.rate('fuzzy')
  s.restartGroup()
  assert.deepEqual(s.cards.value.map((word) => word.id), [2])
})
