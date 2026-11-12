/** 当天临时练习接口；没有题库、素材导入或持久化计分。 */
import { apiGet, apiPost } from './client'
const base = '/api/training/daily'
/** @returns {Promise<object>} 自动准备当天第一组，重复请求共用同一组。 */
export function ensureDaily() { return apiPost(base, {}) }
/** @returns {Promise<object>} 当前词与当天临时进度。 */
export function readDaily() { return apiGet(base) }
/** @param {string} requestId 同一次操作的去重标识 @returns {Promise<object>} 排名靠前的下一批词。 */
export function moreDaily(requestId) { return apiPost(`${base}/more`, { request_id: requestId }) }
/** @param {string} action answer/advance/retry @param {object} data 当前组、单词与选项 @returns {Promise<object>} 最新临时状态。 */
export function dailyAction(action, data) { return apiPost(`${base}/${action}`, data) }
