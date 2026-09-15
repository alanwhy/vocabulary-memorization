<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { apiGet, apiPut } from '@/api/client'

const form = reactive({
  enabled: false,
  api_key: '',
  base_url: '',
  fallback_model: '',
  thinking_model: '',
  tts_api_key: '',
  tts_cluster: '',
  tts_voice_type: '',
  start_color: '#e4f7e9',
  end_color: '#a11d1d',
  segments: 6,
})
const savingSection = ref('')
const sectionMessages = reactive({
  deepseek: { error: '', ok: '' },
  tts: { error: '', ok: '' },
  colors: { error: '', ok: '' },
})

const previewSegments = computed(() => {
  const segments = Number(form.segments)
  const count = Number.isInteger(segments) && segments >= 2 && segments <= 12 ? segments : 2
  return Array.from({ length: count }, (_, index) => index + 1)
})
const previewGradient = computed(() => ({
  background: `linear-gradient(90deg, ${form.start_color}, ${form.end_color})`,
}))

async function loadSettings() {
  const data = await apiGet('/api/admin/settings')
  Object.assign(form, data)
}

async function saveSettings(section) {
  if (savingSection.value) return
  for (const message of Object.values(sectionMessages)) {
    message.error = ''
    message.ok = ''
  }
  savingSection.value = section
  try {
    const data = await apiPut('/api/admin/settings', form)
    Object.assign(form, data)
    sectionMessages[section].ok =
      section === 'colors' ? '渐变设置已应用' : section === 'tts' ? '语音设置已保存' : '查词设置已保存'
  } catch (e) {
    sectionMessages[section].error = e.message || '保存失败'
  } finally {
    savingSection.value = ''
  }
}

onMounted(loadSettings)
</script>

<template>
  <section class="settings-section">
    <div class="section-heading">
      <div>
        <h2>DeepSeek 查词</h2>
        <p>控制单词释义的查询服务。</p>
      </div>
    </div>
    <div class="card settings-card">
    <div class="checkbox-row">
      <input type="checkbox" id="dsEnabled" v-model="form.enabled" />
      <label for="dsEnabled" style="margin: 0">启用 DeepSeek 查词</label>
    </div>
    <label>API Key</label>
    <input type="password" v-model="form.api_key" placeholder="sk-..." />
    <label>Base URL</label>
    <input type="text" v-model="form.base_url" placeholder="https://api.deepseek.com" />
    <label>兜底模型（查询用）</label>
    <input type="text" v-model="form.fallback_model" placeholder="deepseek-v4-flash" />
    <label>思考模型（预留，暂未启用，可留空）</label>
    <input type="text" v-model="form.thinking_model" placeholder="如 deepseek-reasoner" />
      <div class="section-actions">
        <button class="primary" :disabled="!!savingSection" @click="saveSettings('deepseek')">
          {{ savingSection === 'deepseek' ? '保存中…' : '保存查词设置' }}
        </button>
        <p class="section-message" :class="{ error: !!sectionMessages.deepseek.error, ok: !!sectionMessages.deepseek.ok }">
          {{ sectionMessages.deepseek.error || sectionMessages.deepseek.ok }}
        </p>
      </div>
    </div>
  </section>

  <section class="settings-section">
    <div class="section-heading">
      <div>
        <h2>豆包语音</h2>
        <p>用于单词读音的语音合成服务。</p>
      </div>
    </div>
    <div class="card settings-card">
    <label>API Key</label>
    <input type="password" v-model="form.tts_api_key" placeholder="BytePlus Seed Speech 的 API Key" />
    <label>Cluster（集群）</label>
    <input type="text" v-model="form.tts_cluster" placeholder="volcano_tts" />
    <label>音色 VoiceType</label>
    <input type="text" v-model="form.tts_voice_type" placeholder="BV001 或 BV002" />
      <div class="section-actions">
        <button class="primary" :disabled="!!savingSection" @click="saveSettings('tts')">
          {{ savingSection === 'tts' ? '保存中…' : '保存语音设置' }}
        </button>
        <p class="section-message" :class="{ error: !!sectionMessages.tts.error, ok: !!sectionMessages.tts.ok }">
          {{ sectionMessages.tts.error || sectionMessages.tts.ok }}
        </p>
      </div>
    </div>
  </section>

  <section class="settings-section gradient-section">
    <div class="section-heading">
      <div>
        <h2>背诵次数渐变</h2>
        <p>次数徽标与例句命中词会使用同一组颜色。</p>
      </div>
      <span class="live-badge">实时预览</span>
    </div>
    <div class="card gradient-studio">
      <div class="gradient-controls">
        <label class="color-field">
          <span>起始色</span>
          <span class="color-input-wrap"><input type="color" v-model="form.start_color" /><output>{{ form.start_color }}</output></span>
        </label>
        <label class="color-field">
          <span>终止色</span>
          <span class="color-input-wrap"><input type="color" v-model="form.end_color" /><output>{{ form.end_color }}</output></span>
        </label>
        <label class="segment-field">
          <span>分段数</span>
          <span class="segment-input"><input type="number" v-model.number="form.segments" min="2" max="12" step="1" /><small>2–12 段</small></span>
        </label>
      </div>
      <div class="gradient-preview" aria-label="次数渐变预览">
        <div class="preview-copy"><strong>次数越高，颜色越接近终止色</strong><span>{{ previewSegments.length }} 个分段</span></div>
        <div class="preview-track" :style="previewGradient">
          <span v-for="segment in previewSegments" :key="segment">{{ segment }}</span>
        </div>
      </div>
      <div class="section-actions gradient-actions">
        <button class="primary" :disabled="!!savingSection" @click="saveSettings('colors')">
          {{ savingSection === 'colors' ? '应用中…' : '应用渐变设置' }}
        </button>
        <p class="section-message" :class="{ error: !!sectionMessages.colors.error, ok: !!sectionMessages.colors.ok }">
          {{ sectionMessages.colors.error || sectionMessages.colors.ok }}
        </p>
      </div>
    </div>
  </section>
</template>

<style scoped>
.settings-section { margin: 28px 0; }
.section-heading { display: flex; align-items: flex-end; justify-content: space-between; gap: 16px; margin: 0 4px 10px; }
.section-heading h2 { margin: 0; font-size: 17px; letter-spacing: -0.2px; }
.section-heading p { margin: 4px 0 0; color: var(--muted); font-size: 13px; }
.settings-card, .gradient-studio { padding: 20px; }
.settings-card { display: grid; gap: 7px; }
.settings-card label, .color-field, .segment-field { color: var(--muted); font-size: 13px; }
.settings-card input[type='text'], .settings-card input[type='password'] { width: 100%; }
.section-actions { display: flex; align-items: center; gap: 12px; min-height: 34px; margin-top: 10px; }
.section-message { margin: 0; font-size: 13px; color: var(--muted); }
.section-message.ok { color: #2a8b57; }
.section-message.error { color: var(--danger); }
.live-badge { padding: 4px 8px; border-radius: 999px; background: var(--accent-soft); color: var(--accent); font-size: 12px; }
.gradient-studio { display: grid; grid-template-columns: minmax(220px, 0.9fr) minmax(240px, 1.1fr); gap: 24px; align-items: center; }
.gradient-controls { display: grid; gap: 14px; }
.color-field, .segment-field { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.color-input-wrap, .segment-input { display: flex; align-items: center; gap: 8px; }
.color-input-wrap input[type='color'] { width: 42px; height: 32px; padding: 3px; border: 1px solid var(--border); border-radius: 7px; background: var(--card-bg); cursor: pointer; }
.color-input-wrap output { width: 76px; color: var(--text); font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 12px; }
.segment-input input { width: 58px; padding: 7px 8px; border: 1px solid var(--border); border-radius: 7px; background: var(--card-bg); color: var(--text); }
.segment-input small { color: var(--muted); }
.gradient-preview { align-self: stretch; display: flex; flex-direction: column; justify-content: center; gap: 14px; padding: 18px; border: 1px solid var(--border); border-radius: 10px; background: color-mix(in srgb, var(--accent-soft) 42%, var(--card-bg)); }
.preview-copy { display: flex; align-items: baseline; justify-content: space-between; gap: 10px; }
.preview-copy strong { color: var(--text); font-size: 14px; }
.preview-copy span { color: var(--muted); font-size: 12px; white-space: nowrap; }
.preview-track { display: grid; grid-template-columns: repeat(v-bind('previewSegments.length'), minmax(0, 1fr)); gap: 2px; padding: 3px; border-radius: 9px; min-height: 42px; box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.45); }
.preview-track span { display: grid; place-items: center; min-width: 0; border-radius: 5px; background: color-mix(in srgb, var(--card-bg) 36%, transparent); color: var(--text); font-size: 12px; font-weight: 700; text-shadow: 0 1px 2px rgba(255, 255, 255, 0.45); }
.gradient-actions { grid-column: 1 / -1; margin-top: -4px; }
@media (max-width: 560px) {
  .gradient-studio { grid-template-columns: 1fr; gap: 18px; }
  .gradient-actions { margin-top: 0; }
  .preview-copy { align-items: flex-start; flex-direction: column; gap: 3px; }
}
</style>
