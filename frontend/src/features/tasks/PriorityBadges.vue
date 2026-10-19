<script setup lang="ts">
import { priorityTone } from './tasks'
defineProps<{ values: { label: string; value: string }[] }>()
</script>

<template>
  <span class="priority-badges">
    <span v-for="(field, index) in values" :key="index" class="priority-badge" :class="priorityTone(field.value)"
      :title="`${field.label}：${field.value}`" :aria-label="`${field.label}：${field.value}`">
      <span v-if="values.length > 1" class="priority-field">{{ field.label }}：</span>{{ field.value }}
    </span>
  </span>
</template>
