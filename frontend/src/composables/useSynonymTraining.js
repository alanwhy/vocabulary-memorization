/** 一词一题的当天状态，内容仅从服务器内存读取，不写浏览器长期存储。 */
import { reactive, onBeforeUnmount } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { ensureDaily, readDaily, moreDaily, dailyAction } from '@/api/training'

/** @returns {object} 自动准备、作答、下一词和再来一组。 */
export function useSynonymTraining() {
  const auth = useAuthStore(), owner = auth.currentUser?.id
  const state = reactive({ snapshot: null, loading: true, busy: false, error: '' })
  let alive = true, timer, epoch = 0, moreKey = null
  /** @returns {boolean} 防止页面卸载或切换账号后的响应污染当前页面。 */
  function valid() { return alive && auth.currentUser?.id === owner }
  /** @returns {string} 上海日期，用于跨日立即更新页面。 */
  function today() { return new Date(Date.now() + 8 * 3600000).toISOString().slice(0, 10) }
  /** @param {object} snapshot 服务端临时状态 @param {number} requestEpoch 请求版本 @returns {void} 只接收最新当天响应。 */
  function accept(snapshot, requestEpoch) {
    if (!valid() || requestEpoch !== epoch) return
    if (snapshot.date !== today()) { state.snapshot = null; initialize(); return }
    state.snapshot = snapshot
  }
  /** @returns {void} 可见页面定期更新，后台页面停止请求。 */
  function schedule() {
    clearTimeout(timer)
    if (!valid() || document.hidden) return
    timer = setTimeout(poll, state.snapshot?.generating ? 2000 : 15000)
  }
  /** @returns {Promise<void>} 首次、跨日或词库从空变为非空时自动准备。 */
  async function initialize() {
    const requestEpoch = ++epoch
    if (state.snapshot?.date !== today()) state.snapshot = null
    state.loading = !state.snapshot
    state.error = ''
    try { accept(await ensureDaily(), requestEpoch) } catch (error) { if (valid() && epoch === requestEpoch) state.error = error.message } finally { if (valid() && epoch === requestEpoch) state.loading = false; schedule() }
  }
  /** @returns {Promise<void>} 只读生成进度及归档变化，不重复生成当日首组。 */
  async function poll() {
    if (!valid() || state.busy) { schedule(); return }
    if (!state.snapshot?.group_id || state.snapshot.date !== today()) { await initialize(); return }
    const requestEpoch = epoch
    try {
      const snapshot = await readDaily()
      if (!snapshot.group_id && state.snapshot?.group_id) { await initialize(); return }
      accept(snapshot, requestEpoch)
    } catch (error) { if (valid() && epoch === requestEpoch) state.error = error.message }
    schedule()
  }
  /** @param {Function} request 请求动作 @returns {Promise<void>} 操作互斥并保留失败前内容。 */
  async function operate(request) {
    if (!valid() || state.busy) return
    const requestEpoch = ++epoch; state.busy = true; state.error = ''; clearTimeout(timer)
    try { accept(await request(), requestEpoch) } catch (error) {
      if (valid() && epoch === requestEpoch) { state.error = error.message; try { accept(await readDaily(), requestEpoch) } catch { /* 保留现有内容供重试。 */ } }
    } finally { if (valid()) state.busy = false; schedule() }
  }
  /** @param {string|null} optionId 所选答案，不知道时为 null @returns {Promise<void>} 选择后立即显示解释。 */
  async function answer(optionId) { const s = state.snapshot; return operate(() => dailyAction('answer', { group_id: s.group_id, word_id: s.card.word_id, option_id: optionId })) }
  /** @returns {Promise<void>} 继续本组下一词，不改变原词出现次数。 */
  async function next() { const s = state.snapshot; return operate(() => dailyAction('advance', { group_id: s.group_id, word_id: s.card.word_id })) }
  /** @returns {Promise<void>} 重新生成失败的当前词。 */
  async function retry() { const s = state.snapshot; return operate(() => dailyAction('retry', { group_id: s.group_id, word_id: s.card.word_id })) }
  /** @returns {Promise<void>} 同日继续下一批，网络重试共用请求 ID。 */
  async function more() {
    if (!moreKey) moreKey = crypto.randomUUID ? crypto.randomUUID() : Array.from(crypto.getRandomValues(new Uint8Array(12)), n => n.toString(16).padStart(2, '0')).join('')
    return operate(async () => { const result = await moreDaily(moreKey); moreKey = null; return result })
  }
  /** @returns {void} 回到页面时检查日期、新词和归档变化。 */
  function visibility() { clearTimeout(timer); if (!document.hidden && valid()) initialize() }
  document.addEventListener('visibilitychange', visibility)
  onBeforeUnmount(() => { alive = false; clearTimeout(timer); document.removeEventListener('visibilitychange', visibility) })
  return { state, initialize, answer, next, retry, more }
}
