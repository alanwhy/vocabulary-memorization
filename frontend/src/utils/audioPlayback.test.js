import test from 'node:test'
import assert from 'node:assert/strict'
import { createAudioPlayback } from './audioPlayback.js'

/** A deliberately non-aborting transport verifies even late responses cannot play. */
function fixture({ playError } = {}) {
  const calls = [], played = [], revoked = [], audios = [], errors = [], states = []
  const player = createAudioPlayback({
    fetchAudio: (url, request) => new Promise((resolve, reject) => calls.push({ url, request, resolve, reject })),
    createAudio: (url) => {
      const audio = {
        onended: null, onerror: null, paused: false, removed: false, loaded: false,
        play: async () => { if (playError) throw playError; played.push(url) },
        pause() { this.paused = true },
        removeAttribute() { this.removed = true },
        load() { this.loaded = true },
      }
      audios.push(audio)
      return audio
    },
    urls: { createObjectURL: () => `blob:${audios.length}`, revokeObjectURL: (url) => revoked.push(url) },
    onState: (state) => states.push(state), onError: (error) => errors.push(error),
  })
  const resolve = (index) => calls[index].resolve({ ok: true, blob: async () => new Blob(['audio']) })
  const target = (name, sourceId = 'view') => ({ target: name, sourceId, url: `/audio/${name}` })
  return { player, calls, played, revoked, audios, errors, states, resolve, target }
}

test('late A response cannot replace newer B playback', async () => {
  const f = fixture()
  const a = f.player.play(f.target('A'))
  const b = f.player.play(f.target('B'))
  assert.equal(f.calls[0].request.signal.aborted, true)
  f.resolve(1); await b
  f.resolve(0); await a
  assert.equal(f.played.length, 1)
  assert.equal(f.states.at(-1).target, 'B')
  f.player.stop()
  assert.deepEqual(f.revoked, ['blob:0'])
})

test('same target while loading requests once; ended example replay requests again', async () => {
  const f = fixture()
  const first = f.player.play(f.target('example'))
  await f.player.play(f.target('example'))
  assert.equal(f.calls.length, 1)
  f.resolve(0); await first
  f.audios[0].onended()
  assert.deepEqual(f.revoked, ['blob:0'])
  assert.equal(f.states.at(-1).target, '')
  const replay = f.player.play(f.target('example'))
  assert.equal(f.calls.length, 2)
  f.resolve(1); await replay
  f.player.stop()
  assert.equal(f.revoked.length, 2)
})

test('closing another owner leaves playback intact; closing owner releases media', async () => {
  const f = fixture()
  const promise = f.player.play(f.target('word', 'flashcard'))
  f.resolve(0); await promise
  f.player.stop('tooltip')
  assert.equal(f.revoked.length, 0)
  f.player.stop('flashcard')
  assert.equal(f.audios[0].paused && f.audios[0].removed && f.audios[0].loaded, true)
  assert.equal(f.revoked.length, 1)
  f.player.stop('flashcard')
  assert.equal(f.revoked.length, 1)
})

test('stopping a pending request ignores its later failure', async () => {
  const f = fixture()
  const promise = f.player.play(f.target('example'))
  f.player.stop('view')
  f.calls[0].reject(new Error('late network error'))
  await promise
  assert.deepEqual(f.errors, [])
  assert.equal(f.states.at(-1).status, 'idle')
})

test('auto-play permission failure is silent; manual play reports and releases', async () => {
  const error = new Error('permission denied'); error.name = 'NotAllowedError'
  for (const automatic of [true, false]) {
    const f = fixture({ playError: error })
    const promise = f.player.play({ ...f.target('word'), automatic })
    f.resolve(0); await promise
    assert.equal(f.errors.length, automatic ? 0 : 1)
    assert.equal(f.revoked.length, 1)
  }
})

test('a late blob after replacement never creates a media URL', async () => {
  const f = fixture()
  let resolveBlob
  const promise = f.player.play(f.target('A'))
  f.calls[0].resolve({ ok: true, blob: () => new Promise((resolve) => { resolveBlob = resolve }) })
  await Promise.resolve()
  f.player.stop()
  resolveBlob(new Blob(['late']))
  await promise
  assert.equal(f.audios.length, 0)
})

test('provider error is reported safely and current resources are cleared', async () => {
  const f = fixture()
  const promise = f.player.play(f.target('example'))
  f.calls[0].resolve({ ok: false, json: async () => ({ error: '语音服务繁忙' }) })
  await promise
  assert.deepEqual(f.errors, ['语音服务繁忙'])
  assert.equal(f.states.at(-1).status, 'idle')
})
