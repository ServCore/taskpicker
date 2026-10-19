<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { NButton, NCheckbox, NForm, NFormItem, NInput, NSpin } from 'naive-ui'
import { PhCaretLeft, PhSparkle } from '@phosphor-icons/vue'
import { api } from '../../shared/api/api'
import type { DailyAISettings } from '../../shared/types/types'

defineProps<{ embedded?: boolean }>()
const emit = defineEmits<{ close: []; lock: [value: boolean] }>()
const config = ref<DailyAISettings | null>(null)
const baseURL = ref(''), model = ref(''), key = ref(''), clearKey = ref(false)
const loading = ref(true), saving = ref(false), error = ref(''), message = ref('')
const busy = computed(() => loading.value || saving.value)
const configChanged = computed(() => !!config.value && (baseURL.value !== config.value.base_url || model.value !== config.value.model
  || !!key.value || clearKey.value))
watch(() => busy.value || configChanged.value, value => emit('lock', value), { immediate: true })
function setConfig(value: DailyAISettings) {
  config.value = value; baseURL.value = value.base_url; model.value = value.model
  key.value = ''; clearKey.value = false
}
function discard() { if (config.value) setConfig(config.value); error.value = ''; message.value = '' }
async function load() {
  loading.value = true; error.value = ''
  try { setConfig(await api.GetDailyAISettings()) }
  catch (exc) { error.value = String(exc) }
  finally { loading.value = false }
}
async function saveConfig() {
  if (busy.value || !config.value) return
  saving.value = true; error.value = ''; message.value = ''
  try {
    setConfig(await api.SaveDailyAISettings({ base_url: baseURL.value, model: model.value, api_key: key.value, clear_api_key: clearKey.value }))
    message.value = 'AI 设置已保存'
  } catch (exc) { error.value = String(exc) }
  finally { saving.value = false }
}
onMounted(load)
</script>

<template>
  <main class="workspace ai-settings-page" aria-label="AI 设置">
    <header class="workspace-header">
      <div class="breadcrumb"><PhSparkle :size="16" /><template v-if="!embedded"><span>设置</span><span class="breadcrumb-separator" aria-hidden="true">/</span></template><h1>AI 设置</h1></div>
      <NButton v-if="!embedded" size="small" :disabled="busy || configChanged" @click="emit('close')"><template #icon><PhCaretLeft :size="15" /></template>返回</NButton>
    </header>
    <div class="ai-settings-scroll">
      <div class="ai-settings-content">
        <h2>模型连接</h2>
        <p class="ai-intro">配置 AI 服务的连接信息。支持兼容 Chat Completions 的服务及本机模型。</p>
        <div v-if="error" class="ai-error" role="alert">{{ error }} <NButton v-if="!config && !busy" size="tiny" @click="load">重试</NButton></div>
        <div v-if="message" class="ai-message" role="status">{{ message }}</div>
        <NSpin :show="loading">
          <p v-if="!config && !loading" class="ai-intro">设置加载失败，请重试后编辑。</p>
          <template v-if="config">
          <NForm label-placement="top" @submit.prevent="saveConfig">
            <div class="daily-config-row"><NFormItem label="API 基础地址"><NInput v-model:value="baseURL" aria-label="AI API 基础地址" placeholder="服务商提供的地址，含 /v1 等路径" :disabled="busy" /></NFormItem><NFormItem label="模型名称"><NInput v-model:value="model" aria-label="AI 模型名称" placeholder="填写服务商的模型 ID" :disabled="busy" /></NFormItem></div>
            <NFormItem label="API Key"><NInput v-model:value="key" type="password" show-password-on="click" aria-label="AI API Key" :placeholder="config?.has_api_key ? '已保存，留空保持原 Key' : '填写 API Key；本机免鉴权服务可留空'" :disabled="busy || clearKey" :input-props="{ autocomplete: 'new-password' }" /></NFormItem>
            <p class="daily-config-help">Key 仅保存在这台 Mac，不在界面回显；更换 API 地址需重新填写或清除 Key。</p>
            <NCheckbox v-model:checked="clearKey" :disabled="busy">清除已保存的 Key</NCheckbox>
            <div class="daily-config-actions"><NButton :disabled="busy" @click="discard">放弃修改</NButton><NButton type="primary" attr-type="submit" :loading="saving" :disabled="busy && !saving">保存 AI 设置</NButton></div>
            <p v-if="configChanged" class="daily-config-help">设置有未保存的修改，请保存或放弃后切换页面。</p>
          </NForm>
          </template>
        </NSpin>
      </div>
    </div>
    <footer class="ai-settings-footer">点击「AI 生成日报」时，仅发送所选日期的任务文字与操作记录至配置的地址。</footer>
  </main>
</template>

<style scoped>
.ai-settings-scroll { flex: 1; min-height: 0; overflow: auto; }
.ai-settings-content { max-width: 850px; margin: 0 auto; padding: 28px 36px; }
.ai-settings-content h2 { font-size: 16px; font-weight: 600; margin: 0 0 8px; }
.ai-intro { color: var(--muted); font-size: 12px; line-height: 1.8; margin: 0 0 26px; }
.ai-settings-footer { border-top: 1px solid var(--line); padding: 14px 28px; color: var(--subtle); font-size: 11px; line-height: 1.8; }
.ai-error, .ai-message { padding: 10px 12px; margin-bottom: 20px; border-radius: 5px; font-size: 12px; overflow-wrap: anywhere; }
.ai-error { color: var(--danger); background: #fff2f0; }
.ai-message { color: var(--success); background: #edf8f0; }
.daily-config-row { display: grid; grid-template-columns: 1fr 1fr; gap: 20px; }
.daily-config-help { color: var(--subtle); font-size: 11px; line-height: 1.8; margin: -6px 0 14px; }
.daily-config-actions { display: flex; align-items: center; justify-content: flex-end; gap: 9px; margin-block: 28px 14px; }

@media (max-width: 900px) { .daily-config-row { grid-template-columns: 1fr; gap: 0; } .ai-settings-content { padding: 24px; } .daily-config-actions { flex-wrap: wrap; } }
</style>
