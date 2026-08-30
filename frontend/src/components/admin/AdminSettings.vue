<script setup>
import { onMounted, reactive, ref } from 'vue'
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
})
const savingSettings = ref(false)
const settingsMsgError = ref('')
const settingsMsgOk = ref('')

async function loadSettings() {
  const data = await apiGet('/api/admin/settings')
  Object.assign(form, data)
}

async function saveSettings() {
  settingsMsgError.value = ''
  settingsMsgOk.value = ''
  if (savingSettings.value) return
  savingSettings.value = true
  try {
    const data = await apiPut('/api/admin/settings', form)
    Object.assign(form, data)
    settingsMsgOk.value = '已保存'
  } catch (e) {
    settingsMsgError.value = e.message || '保存失败'
  } finally {
    savingSettings.value = false
  }
}

onMounted(loadSettings)
</script>

<template>
  <h2>DeepSeek 查词配置</h2>
  <div class="card">
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
  </div>

  <h2>豆包语音配置（读音）</h2>
  <div class="card">
    <label>API Key</label>
    <input type="password" v-model="form.tts_api_key" placeholder="BytePlus Seed Speech 的 API Key" />
    <label>Cluster（集群）</label>
    <input type="text" v-model="form.tts_cluster" placeholder="volcano_tts" />
    <label>音色 VoiceType</label>
    <input type="text" v-model="form.tts_voice_type" placeholder="BV001 或 BV002" />
    <button class="primary" :disabled="savingSettings" @click="saveSettings">保存配置</button>
    <p class="msg" :class="{ error: !!settingsMsgError, ok: !!settingsMsgOk }">
      {{ settingsMsgError || settingsMsgOk }}
    </p>
  </div>
</template>
