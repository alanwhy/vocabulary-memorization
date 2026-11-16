// Word audio is persisted by the server; example audio lives only for the current playback.
import { reactive } from 'vue'
import { ElMessage } from 'element-plus'
import { getToken } from '@/api/client'
import { createAudioPlayback } from '@/utils/audioPlayback'

export const speechState = reactive({ status: 'idle', target: '', sourceId: '' })
const player = createAudioPlayback({
  fetchAudio: (url, options) => fetch(url, options),
  createAudio: (url) => new Audio(url),
  urls: URL,
  onState: (state) => Object.assign(speechState, state),
  onError: (message) => ElMessage.error(message),
})

/** Attach the existing application Bearer token without exposing provider credentials. */
function authHeaders() {
  const token = getToken()
  return token ? { Authorization: `Bearer ${token}` } : {}
}

/** Play a canonical word using the shared server cache. Options identify the owning view and auto-play intent. */
export function speakWord(wordKey, options = {}) {
  if (!wordKey) return
  const key = wordKey.trim().toLowerCase()
  return player.play({
    target: `word:${key}`,
    sourceId: options.sourceId || `word:${key}`,
    automatic: options.automatic || false,
    url: `/api/pronounce/${encodeURIComponent(key)}`,
    request: { headers: authHeaders() },
  })
}

/** Synthesize only the English original, requesting no browser or server persistence. */
export function speakExample(text, options = {}) {
  if (!text?.trim()) return
  const original = text.trim()
  return player.play({
    target: `example:${original}`,
    sourceId: options.sourceId || 'example',
    url: '/api/pronounce/example',
    request: {
      method: 'POST',
      cache: 'no-store',
      headers: { ...authHeaders(), 'Content-Type': 'application/json' },
      body: JSON.stringify({ text: original }),
    },
  })
}

/** Report the loading state only for the matching target and view. */
export function isSpeechLoading(target, sourceId) {
  return speechState.status === 'loading' && speechState.target === target && speechState.sourceId === sourceId
}

/** Release an owning view's player, leaving another view's newer playback intact. */
export function stopSpeech(sourceId) { player.stop(sourceId) }
