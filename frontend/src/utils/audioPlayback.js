/** A single page-wide player: only current requests may play, and no completed audio is retained. */
export function createAudioPlayback({ fetchAudio, createAudio, urls, onState = () => {}, onError = () => {} }) {
  let active = null
  let sequence = 0

  /** Release media and its URL exactly once, including unfinished requests. */
  function release(entry) {
    entry.controller.abort()
    if (entry.audio) {
      entry.audio.onended = null
      entry.audio.onerror = null
      entry.audio.pause()
      entry.audio.removeAttribute('src')
      entry.audio.load()
    }
    if (entry.url) urls.revokeObjectURL(entry.url)
    entry.audio = null
    entry.url = null
  }

  /** Stop only the owning view's audio, or all audio when no source is supplied. */
  function stop(sourceId) {
    if (!active || sourceId !== undefined && sourceId !== active.sourceId) return
    sequence++
    release(active)
    active = null
    onState({ status: 'idle', target: '', sourceId: '' })
  }

  /** Fetch and play one target; superseded responses and aborts never update UI or report errors. */
  async function play({ target, sourceId, url, request = {}, automatic = false }) {
    if (active?.target === target && active.sourceId === sourceId && active.status === 'loading') return
    stop()
    const entry = { target, sourceId, status: 'loading', controller: new AbortController(), audio: null, url: null }
    const epoch = ++sequence
    active = entry
    onState({ status: entry.status, target, sourceId })
    const current = () => active === entry && sequence === epoch
    try {
      const response = await fetchAudio(url, { ...request, signal: entry.controller.signal })
      if (!current()) return
      if (!response.ok) {
        const data = await response.json().catch(() => ({}))
        if (current()) throw new Error(data.error || '语音请求失败')
        return
      }
      const blob = await response.blob()
      if (!current()) return
      entry.url = urls.createObjectURL(blob)
      entry.audio = createAudio(entry.url)
      entry.audio.onended = () => { if (current()) stop(sourceId) }
      entry.audio.onerror = () => {
        if (!current()) return
        stop(sourceId)
        onError('播放失败，请稍后重试')
      }
      await entry.audio.play()
      if (!current()) return
      entry.status = 'playing'
      onState({ status: entry.status, target, sourceId })
    } catch (error) {
      if (!current()) return
      stop(sourceId)
      if (error?.name !== 'AbortError' && !(automatic && error?.name === 'NotAllowedError')) {
        onError(error.message || '播放失败，请稍后重试')
      }
    }
  }

  return { play, stop }
}
