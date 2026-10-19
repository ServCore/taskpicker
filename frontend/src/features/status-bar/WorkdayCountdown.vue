<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { PhCoffee, PhTimer } from '@phosphor-icons/vue'
import { countdownText, workdayState } from './workday'

const state = ref(workdayState())
let timer: ReturnType<typeof setTimeout> | undefined
let stopped = false
function refresh() {
  clearTimeout(timer)
  if (stopped) return
  const now = new Date()
  state.value = workdayState(now)
  if (document.hidden) return
  // Count down against wall-clock time, so sleep and clock changes cannot drift.
  // Static lunch/off-hours labels only need a timer for their next boundary.
  const delay = state.value.remainingSeconds === null ? state.value.nextChange - now.getTime() : 1000 - now.getMilliseconds()
  timer = setTimeout(refresh, Math.max(1, delay))
}
onMounted(() => {
  window.addEventListener('focus', refresh)
  document.addEventListener('visibilitychange', refresh)
  refresh()
})
onUnmounted(() => {
  stopped = true
  clearTimeout(timer)
  window.removeEventListener('focus', refresh)
  document.removeEventListener('visibilitychange', refresh)
})
</script>

<template>
  <div class="workday-countdown" :class="state.phase" role="timer" aria-live="off"
    :aria-label="`${state.label}${state.remainingSeconds === null ? '' : ` ${countdownText(state.remainingSeconds)}`}`"
    title="按本机时间：08:30–11:30 距离午饭；11:30–13:30 午休；13:30–17:30 距离下班">
    <PhCoffee v-if="state.phase === 'lunch'" :size="14" /><PhTimer v-else :size="14" />
    <span>{{ state.label }}</span>
    <time v-if="state.remainingSeconds !== null" :datetime="`PT${state.remainingSeconds}S`">{{ countdownText(state.remainingSeconds) }}</time>
  </div>
</template>

<style scoped>
.workday-countdown { display: flex; flex-shrink: 0; align-items: center; gap: 6px; padding-left: 16px; margin-left: 8px; border-left: 1px solid var(--line); white-space: nowrap; color: var(--muted); }
.workday-countdown time { font-variant-numeric: tabular-nums; font-family: 'SF Mono', Menlo, monospace; font-size: 11px; color: var(--primary-pressed); }
.workday-countdown.lunch { color: var(--success); }
.workday-countdown.off { color: var(--subtle); }
</style>
