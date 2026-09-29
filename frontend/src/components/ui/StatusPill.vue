<script setup lang="ts">
import { computed } from 'vue';

import type { QuestionnaireStatus } from '@/api/types';

const props = defineProps<{ status: QuestionnaireStatus | string; size?: 'sm' | 'md' }>();

const MAP: Record<string, { label: string; cls: string }> = {
  published: {
    label: '已发布',
    cls: 'bg-emerald-50 text-emerald-700 ring-emerald-600/20 dark:bg-emerald-500/10 dark:text-emerald-300 dark:ring-emerald-400/25',
  },
  draft: {
    label: '草稿',
    cls: 'bg-amber-50 text-amber-700 ring-amber-600/20 dark:bg-amber-500/10 dark:text-amber-300 dark:ring-amber-400/25',
  },
  closed: {
    label: '已关闭',
    cls: 'bg-slate-100 text-slate-600 ring-slate-500/20 dark:bg-slate-500/10 dark:text-slate-300 dark:ring-slate-400/25',
  },
};

const meta = computed(
  () => MAP[props.status] ?? { label: props.status, cls: 'bg-sunken text-muted ring-line' },
);
</script>

<template>
  <span
    class="inline-flex items-center gap-1.5 rounded-full font-medium ring-1 ring-inset whitespace-nowrap"
    :class="[meta.cls, size === 'sm' ? 'px-2 py-0.5 text-[11px]' : 'px-2.5 py-1 text-xs']"
  >
    <span class="size-1.5 rounded-full bg-current opacity-70" />
    {{ meta.label }}
  </span>
</template>
