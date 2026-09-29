<script setup lang="ts">
import { computed } from 'vue';

const props = withDefaults(
  defineProps<{
    variant?: 'text' | 'card' | 'circle' | 'rect';
    lines?: number;
  }>(),
  { variant: 'text', lines: 1 },
);

// The trailing line is shortened so a stack of bars reads as a paragraph
// rather than as a solid block.
const rows = computed(() => Array.from({ length: Math.max(1, props.lines) }, (_, i) => i));
</script>

<template>
  <div v-if="variant === 'card'" class="card p-4 sm:p-5" aria-hidden="true">
    <div class="skeleton h-4 w-1/3 rounded-full" />
    <div class="mt-3.5 space-y-2.5">
      <div class="skeleton h-3 w-full rounded-full" />
      <div class="skeleton h-3 w-4/5 rounded-full" />
    </div>
  </div>

  <div v-else-if="variant === 'text'" class="w-full space-y-2" aria-hidden="true">
    <div
      v-for="(i, index) in rows"
      :key="i"
      class="skeleton h-3.5 rounded-full"
      :class="index === rows.length - 1 && rows.length > 1 ? 'w-3/5' : 'w-full'"
    />
  </div>

  <div v-else-if="variant === 'circle'" class="skeleton size-10 shrink-0 rounded-full" aria-hidden="true" />

  <div v-else class="skeleton h-24 w-full rounded-[var(--radius-control)]" aria-hidden="true" />
</template>
