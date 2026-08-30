// 单词发音播放：请求后端 /api/pronounce/{wordKey} 拿豆包 TTS 合成的音频。
// 豆包语音是唯一读音来源——未配置或合成失败时明确提示，不做任何本地合成回退。
import { ElMessage } from 'element-plus'
import { getToken } from '@/api/client'

let activeAudio = null

export async function speakWord(wordKey) {
  if (!wordKey) return
  try {
    const token = getToken()
    const res = await fetch(`/api/pronounce/${encodeURIComponent(wordKey)}`, {
      headers: token ? { Authorization: `Bearer ${token}` } : {},
    })
    if (!res.ok) {
      const data = await res.json().catch(() => ({}))
      ElMessage.error(data.error || '发音失败')
      return
    }
    const blob = await res.blob()
    if (activeAudio) {
      activeAudio.pause()
    }
    const url = URL.createObjectURL(blob)
    const audio = new Audio(url)
    activeAudio = audio
    audio.play().catch(() => ElMessage.error('播放失败'))
  } catch {
    ElMessage.error('发音失败，请稍后重试')
  }
}
