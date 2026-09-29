<script setup lang="ts">
import { Loader2 } from 'lucide-vue-next';

withDefaults(
  defineProps<{
    size?: 'sm' | 'md' | 'lg';
    label?: string;
  }>(),
  { size: 'md' },
);

const SIZES = {
  sm: 'size-4',
  md: 'size-6',
  lg: 'size-9',
} as const;
</script>

<template>
  <div
    class="inline-flex flex-col items-center gap-2 text-subtle"
    role="status"
    :aria-label="label ?? '加载中'"
  >
    <Loader2 class="animate-spin" :class="SIZES[size]" aria-hidden="true" />
    <span v-if="label" class="text-xs">{{ label }}</span>
  </div>
</template>
