<script setup lang="ts">
import { CheckCircle2, Info, X, XCircle } from 'lucide-vue-next';

import { dismiss, toasts, type ToastKind } from '@/lib/toast';

const ICONS = { success: CheckCircle2, error: XCircle, info: Info } as const;

const TONE: Record<ToastKind, string> = {
  success: 'text-emerald-600 dark:text-emerald-400',
  error: 'text-rose-600 dark:text-rose-400',
  info: 'text-brand-600 dark:text-brand-400',
};
</script>

<template>
  <div
    class="pointer-events-none fixed inset-x-0 bottom-0 z-[100] flex flex-col items-center gap-2 p-4 sm:items-end sm:p-6"
    aria-live="polite"
    aria-atomic="false"
  >
    <TransitionGroup
      enter-active-class="transition duration-300 ease-[var(--ease-spring)]"
      enter-from-class="opacity-0 translate-y-3 sm:translate-x-3 sm:translate-y-0"
      leave-active-class="transition duration-200 ease-in absolute"
      leave-to-class="opacity-0 scale-95"
      move-class="transition duration-200"
    >
      <div
        v-for="t in toasts"
        :key="t.id"
        class="pointer-events-auto card w-full max-w-sm px-4 py-3 flex items-start gap-3 shadow-lg"
      >
        <component :is="ICONS[t.kind]" :class="['size-5 shrink-0 mt-0.5', TONE[t.kind]]" />
        <div class="min-w-0 flex-1">
          <p class="text-sm font-medium leading-snug">{{ t.title }}</p>
          <p v-if="t.detail" class="mt-0.5 text-xs text-muted break-words">{{ t.detail }}</p>
        </div>
        <button
          type="button"
          class="-m-1 shrink-0 rounded p-1 text-subtle transition hover:text-ink"
          aria-label="关闭"
          @click="dismiss(t.id)"
        >
          <X class="size-4" />
        </button>
      </div>
    </TransitionGroup>
  </div>
</template>
