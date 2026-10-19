<script setup lang="ts">
import { ref, watch } from 'vue'
import { NAlert, NSwitch } from 'naive-ui'
import { PhFolderSimple } from '@phosphor-icons/vue'
import { api } from '../../shared/api/api'
import type { Project } from '../../shared/types/types'

defineProps<{ projects: Project[] }>()
const emit = defineEmits<{ updated: [projects: Project[]]; lock: [value: boolean] }>()
const savingID = ref(''), error = ref('')
watch(savingID, value => emit('lock', !!value), { flush: 'sync' })
async function toggle(project: Project, enabled: boolean) {
  if (savingID.value) return
  savingID.value = project.id; error.value = ''
  try { emit('updated', await api.SetProjectEnabled(project.id, enabled)) }
  catch (exc) { error.value = String(exc) }
  finally { savingID.value = '' }
}
</script>

<template>
  <main class="workspace projects-settings-page" aria-label="项目管理">
    <header class="workspace-header"><div class="breadcrumb"><PhFolderSimple :size="17" /><h1>项目管理</h1></div></header>
    <div class="projects-content">
      <h2>启用项目</h2>
      <p class="projects-help">停用后，项目及其文档不会显示在左侧栏目；数据保留，可随时重新启用。</p>
      <NAlert v-if="error" type="error" :show-icon="false" role="alert">{{ error }}</NAlert>
      <p v-if="!projects.length" class="projects-empty">暂无项目，可在左侧栏目新建项目。</p>
      <article v-for="project in projects" :key="project.id" class="project-item">
        <div class="project-info"><strong>{{ project.name }}</strong><span>{{ project.document_keys.length }} 份文档</span></div>
        <div class="project-toggle"><span>{{ project.enabled !== false ? '已启用' : '已停用' }}</span><NSwitch :value="project.enabled !== false" :aria-label="`启用项目 ${project.name}`" :loading="savingID === project.id" :disabled="!!savingID" @update:value="toggle(project, $event)" /></div>
      </article>
    </div>
  </main>
</template>

<style scoped>
.projects-content { flex: 1; min-height: 0; overflow: auto; padding: 24px; }
h2 { margin: 0 0 6px; font-size: 16px; }
.projects-help { margin: 0 0 22px; color: var(--muted); font-size: 12px; line-height: 1.7; }
.project-item { display: flex; align-items: center; justify-content: space-between; gap: 20px; padding: 20px 0; border-bottom: 1px solid var(--line); }
.project-info { min-width: 0; display: flex; flex-direction: column; gap: 6px; }
.project-info strong { font-size: 14px; overflow-wrap: anywhere; }
.project-info span, .project-toggle, .projects-empty { color: var(--muted); font-size: 12px; }
.project-toggle { display: flex; align-items: center; flex-shrink: 0; gap: 10px; }
.projects-empty { padding: 60px 0; text-align: center; }
</style>
