<script setup lang="ts">
import { computed, ref } from 'vue'
import { NAlert, NButton, NForm, NFormItem, NInput, NModal, NSwitch } from 'naive-ui'
import { PhFolderOpen, PhPlus, PhX } from '@phosphor-icons/vue'
import { api } from '../../shared/api/api'
import type { Project } from '../../shared/types/types'

const props = defineProps<{ project: Project | null }>()
const emit = defineEmits<{ close: []; saved: [projects: Project[]] }>()
const name = ref(props.project?.name ?? '')
const originalPaths = props.project?.local_paths ?? (props.project?.local_path ? [props.project.local_path] : [])
const paths = ref(originalPaths.length ? [...originalPaths] : [''])
const originalDailyEnabled = props.project ? props.project.daily_report_enabled ?? true : false
const dailyEnabled = ref(originalDailyEnabled)
const selectedPaths = computed(() => [...new Set(paths.value.map(path => path.trim()).filter(Boolean))])
const busy = ref(false)
const error = ref('')
const changed = computed(() => name.value !== (props.project?.name ?? '') || JSON.stringify(selectedPaths.value) !== JSON.stringify(originalPaths) || dailyEnabled.value !== originalDailyEnabled)
async function pickDirectory(index: number) {
  if (busy.value) return
  busy.value = true; error.value = ''
  try { const path = await api.PickProjectDirectory(paths.value[index] ?? ''); if (path) paths.value[index] = path }
  catch (exc) { error.value = String(exc) }
  finally { busy.value = false }
}
async function save() {
  if (busy.value) return
  if (!name.value.trim()) { error.value = '请填写项目名称。'; return }
  busy.value = true; error.value = ''
  try {
    const projects = await api.SaveProject({ id: props.project?.id ?? '', name: name.value.trim(), local_path: selectedPaths.value[0] ?? '', local_paths: selectedPaths.value, daily_report_enabled: dailyEnabled.value, document_keys: [...(props.project?.document_keys ?? [])] })
    emit('saved', projects)
  } catch (exc) { error.value = String(exc) }
  finally { busy.value = false }
}
</script>

<template>
  <NModal :show="true" :mask-closable="false" :close-on-esc="false">
    <div class="project-panel" role="dialog" aria-modal="true" aria-label="项目设置">
      <header><div><h2>{{ project ? '项目设置' : '新建项目' }}</h2><p>把相关文档与本地项目放在一起</p></div><button class="icon-button" aria-label="返回项目列表" :disabled="busy" @click="emit('close')"><PhX :size="19" /></button></header>
      <NForm label-placement="top" @submit.prevent="save">
        <div class="project-form-body">
          <NFormItem label="项目名称"><NInput v-model:value="name" :maxlength="80" placeholder="例如：yiqiban" :disabled="busy" :input-props="{ 'aria-label': '项目名称' }" /></NFormItem>
          <div class="project-directories">
            <label>本地项目目录 <small>可添加多个</small></label>
            <div v-for="(_, index) in paths" :key="index" class="project-path-field">
              <NInput v-model:value="paths[index]" clearable placeholder="粘贴完整目录路径" :disabled="busy" :input-props="{ 'aria-label': index ? `本地项目目录 ${index + 1}` : '本地项目目录' }" />
              <NButton :disabled="busy" @click="pickDirectory(index)"><template #icon><PhFolderOpen :size="17" /></template>选择目录</NButton>
              <button class="icon-button" :disabled="busy" :aria-label="`移除目录 ${index + 1}`" @click="paths.splice(index, 1)"><PhX :size="16" /></button>
            </div>
            <NButton size="small" quaternary :disabled="busy" @click="paths.push('')"><template #icon><PhPlus :size="15" /></template>添加目录</NButton>
            <p class="project-help">例如分别绑定前端、后端目录；移除目录只解除关联。</p>
          </div>
          <div class="project-daily-option"><div><label for="project-daily-enabled">启用工作日报</label><p>关闭后，该项目的记录不参与日报整理，已有日报正文保留。</p></div><NSwitch v-model:value="dailyEnabled" :disabled="busy" id="project-daily-enabled" aria-label="启用工作日报" /></div>
          <NAlert v-if="error" type="error" :show-icon="false" role="alert">{{ error }}</NAlert>
        </div>
        <footer><div><NButton :disabled="busy" @click="emit('close')">返回</NButton><small>{{ changed ? '直接返回不会保存' : '项目设置保存在本机' }}</small></div><NButton attr-type="submit" type="primary" :loading="busy" :disabled="busy || (!!project && !changed)">{{ project ? '保存项目' : '创建项目' }}</NButton></footer>
      </NForm>
    </div>
  </NModal>
</template>

<style scoped>
.project-panel { width: min(660px, calc(100vw - 48px)); max-height: calc(100vh - 64px); overflow: auto; border-radius: 10px; background: #fff; box-shadow: 0 12px 48px #1f232926; }
header { display: flex; align-items: flex-start; justify-content: space-between; padding: 22px 24px 16px; border-bottom: 1px solid var(--line); }
h2 { margin: 0; font-size: 17px; font-weight: 600; }
header p { margin: 5px 0 0; color: var(--muted); font-size: 12px; }
.project-form-body { padding: 22px 24px; }
.project-path-field { display: flex; align-items: center; width: 100%; gap: 8px; margin-top: 9px; }
.project-path-field .n-input { flex: 1; min-width: 0; }
.project-path-field .n-button { flex-shrink: 0; height: 36px; }
.project-directories { margin-bottom: 20px; }
.project-directories > label { display: block; font-weight: 500; }
.project-directories small { font-weight: 400; font-size: 11px; margin-left: 6px; color: var(--muted); }
.project-directories > .n-button { margin-top: 8px; }
.project-directories .project-help { margin: 4px 0 0; }
.project-daily-option { display: flex; align-items: center; gap: 16px; justify-content: space-between; padding: 14px 0; margin-bottom: 16px; border-block: 1px solid var(--line); }
.project-daily-option p { font-size: 12px; color: var(--muted); margin: 4px 0 0; line-height: 1.6; }
.project-help { color: var(--muted); font-size: 12px; line-height: 1.6; margin: -8px 0 22px; }
.project-form-body .n-alert { margin-top: 16px; }
footer { display: flex; align-items: center; justify-content: space-between; padding: 16px 24px; border-top: 1px solid var(--line); }
footer > div { display: flex; align-items: center; gap: 10px; }
footer small { font-size: 11px; color: var(--muted); }
</style>
