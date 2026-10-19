<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { api } from '../../shared/api/api'
import type { ResourceUsage } from '../../shared/types/types'

const usage = ref<ResourceUsage | null>(null)
let timer: ReturnType<typeof setTimeout> | undefined
let stopped = false

async function refresh() {
  try {
    const sample = await api.GetResourceUsage()
    if (!stopped) usage.value = sample
  } catch {
    if (!stopped) usage.value = null
  } finally {
    // Schedule only after the previous request finishes, so calls never overlap.
    if (!stopped) timer = setTimeout(refresh, 2000)
  }
}
onMounted(refresh)
onUnmounted(() => { stopped = true; clearTimeout(timer) })
</script>

<template>
  <div class="resource-usage" aria-label="应用主进程资源占用"
    title="应用主进程占用，每 2 秒刷新；不含 WebView 辅助进程。CPU 100% 表示占满一个核心；内存为当前驻留内存。">
    <span>CPU <b>{{ usage?.cpu_percent == null ? '—' : `${usage.cpu_percent.toFixed(1)}%` }}</b></span>
    <span>内存 <b>{{ usage ? `${(usage.memory_bytes / 1_000_000).toFixed(1)} MB` : '—' }}</b></span>
  </div>
</template>

<style scoped>
.resource-usage { display: flex; align-items: center; gap: 16px; margin-left: 16px; padding-left: 16px; border-left: 1px solid var(--line); white-space: nowrap; }
.resource-usage span { display: flex; align-items: baseline; gap: 6px; }
.resource-usage b { min-width: 5ch; color: var(--muted); font-weight: 400; font-variant-numeric: tabular-nums; }
</style>
