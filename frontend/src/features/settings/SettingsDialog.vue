<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { NModal } from 'naive-ui'
import { PhClock, PhFolderSimple, PhGear, PhSparkle, PhX } from '@phosphor-icons/vue'
import type { Document, Project, SyncSchedule } from '../../shared/types/types'
import AISettingsPage from '../ai-settings/AISettingsPage.vue'
import SyncSchedulesPage from '../schedules/SyncSchedulesPage.vue'
import ProjectsSettingsPage from '../projects/ProjectsSettingsPage.vue'

defineProps<{ projects: Project[]; documents: Document[]; schedules: SyncSchedule[]; runningID: string; blocked: boolean; loadError: string }>()
const emit = defineEmits<{ close: []; lock: [value: boolean]; updated: [items: SyncSchedule[]]; projectsUpdated: [projects: Project[]]; refresh: []; run: [id: string] }>()
const section = ref<'ai' | 'schedules' | 'projects'>('ai')
const aiLocked = ref(true), scheduleLocked = ref(false)
const projectsLocked = ref(false)
const locked = computed(() => aiLocked.value || scheduleLocked.value || projectsLocked.value)
watch(locked, value => emit('lock', value), { immediate: true })
function close() { if (!locked.value) emit('close') }
function select(value: 'ai' | 'schedules' | 'projects') {
  if (locked.value || section.value === value) return
  if (value === 'ai') aiLocked.value = true
  section.value = value
}
</script>

<template>
  <NModal :show="true" preset="card" class="software-settings-shell" :bordered="false" :closable="false" :content-style="{ padding: 0 }" :auto-focus="false" :mask-closable="!locked" :close-on-esc="!locked" @update:show="value => { if (!value) close() }">
    <section class="software-settings-modal" role="dialog" aria-modal="true" aria-label="软件设置">
      <button class="icon-button settings-close" aria-label="关闭软件设置" :disabled="locked" @click="close"><PhX :size="19" /></button>
      <aside class="settings-sidebar">
        <h2><PhGear :size="19" />设置</h2>
        <nav aria-label="设置目录">
          <button :class="{ active: section === 'ai' }" :aria-current="section === 'ai' ? 'page' : undefined" :disabled="locked" @click="select('ai')"><PhSparkle :size="18" />AI 设置</button>
          <button :class="{ active: section === 'schedules' }" :aria-current="section === 'schedules' ? 'page' : undefined" :disabled="locked" @click="select('schedules')"><PhClock :size="18" />定时任务</button>
          <button :class="{ active: section === 'projects' }" :aria-current="section === 'projects' ? 'page' : undefined" :disabled="locked" @click="select('projects')"><PhFolderSimple :size="18" />项目管理</button>
        </nav>
      </aside>
      <div class="settings-panel">
        <AISettingsPage v-if="section === 'ai'" embedded @lock="aiLocked = $event" />
        <ProjectsSettingsPage v-else-if="section === 'projects'" :projects="projects" @updated="emit('projectsUpdated', $event)" @lock="projectsLocked = $event" />
        <SyncSchedulesPage v-else embedded :projects="projects" :documents="documents" :schedules="schedules" :runningID="runningID" :blocked="blocked" :load-error="loadError" @updated="emit('updated', $event)" @refresh="emit('refresh')" @run="emit('run', $event)" @lock="scheduleLocked = $event" />
      </div>
    </section>
  </NModal>
</template>

<style scoped>
.software-settings-shell { width: min(960px, calc(100vw - 48px)); overflow: hidden; border-radius: 10px; }
.software-settings-modal { position: relative; display: grid; grid-template-columns: 176px minmax(0, 1fr); height: min(700px, calc(100vh - 64px)); overflow: hidden; background: #fff; }
.settings-sidebar { min-height: 0; overflow: auto; background: var(--surface); border-right: 1px solid var(--line); padding: 22px 12px; }
.settings-sidebar h2 { display: flex; align-items: center; gap: 10px; margin: 0 10px 24px; font-size: 16px; font-weight: 600; }
.settings-sidebar nav { display: flex; flex-direction: column; gap: 6px; }
.settings-sidebar nav button { display: flex; align-items: center; gap: 10px; width: 100%; padding: 11px 12px; border: 0; border-radius: 6px; background: transparent; text-align: left; font-size: 13px; color: var(--text); }
.settings-sidebar nav button:hover:not(:disabled) { background: var(--hover); }
.settings-sidebar nav button.active { background: #e0e9ff; color: var(--primary-pressed); }
.settings-panel { display: flex; min-width: 0; min-height: 0; container-type: inline-size; }
.settings-close { position: absolute; z-index: 1; top: 18px; right: 18px; }
.software-settings-modal :deep(.workspace) { flex: 1; }
.software-settings-modal :deep(.workspace-header) { min-height: 66px; padding-right: 64px; }
.software-settings-modal :deep(.ai-settings-content), .software-settings-modal :deep(.schedules-content) { padding: 24px; }
@container (max-width: 540px) { .settings-panel :deep(.daily-config-row) { grid-template-columns: 1fr; gap: 0; } }
@media (max-width: 760px) { .software-settings-modal { grid-template-columns: 146px minmax(0, 1fr); } }
</style>
