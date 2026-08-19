<script setup>
import { onMounted, reactive, ref } from 'vue'
import { apiGet, apiPut } from '@/api/client'

const dsForm = reactive({
  enabled: false,
  api_key: '',
  base_url: '',
  fallback_model: '',
  thinking_model: '',
})
const savingSettings = ref(false)
const settingsMsgError = ref('')
const settingsMsgOk = ref('')

async function loadSettings() {
  const data = await apiGet('/api/admin/settings')
  dsForm.enabled = data.enabled
  dsForm.api_key = data.api_key
  dsForm.base_url = data.base_url
  dsForm.fallback_model = data.fallback_model
  dsForm.thinking_model = data.thinking_model
}

async function saveSettings() {
  settingsMsgError.value = ''
  settingsMsgOk.value = ''
  if (savingSettings.value) return
  savingSettings.value = true
  try {
    const data = await apiPut('/api/admin/settings', dsForm)
    dsForm.enabled = data.enabled
    dsForm.api_key = data.api_key
    dsForm.base_url = data.base_url
    dsForm.fallback_model = data.fallback_model
    dsForm.thinking_model = data.thinking_model
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
      <input type="checkbox" id="dsEnabled" v-model="dsForm.enabled" />
      <label for="dsEnabled" style="margin: 0">启用 DeepSeek 查词</label>
    </div>
    <label>API Key</label>
    <input type="password" v-model="dsForm.api_key" placeholder="sk-..." />
    <label>Base URL</label>
    <input type="text" v-model="dsForm.base_url" placeholder="https://api.deepseek.com" />
    <label>兜底模型（查询用）</label>
    <input type="text" v-model="dsForm.fallback_model" placeholder="deepseek-v4-flash" />
    <label>思考模型（预留，暂未启用，可留空）</label>
    <input type="text" v-model="dsForm.thinking_model" placeholder="如 deepseek-reasoner" />
    <button class="primary" :disabled="savingSettings" @click="saveSettings">保存配置</button>
    <p class="msg" :class="{ error: !!settingsMsgError, ok: !!settingsMsgOk }">
      {{ settingsMsgError || settingsMsgOk }}
    </p>
  </div>
</template>
