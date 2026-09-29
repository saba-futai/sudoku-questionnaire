<script setup lang="ts">
import { AlertTriangle } from 'lucide-vue-next';

import AppButton from './AppButton.vue';
import AppModal from './AppModal.vue';

withDefaults(
  defineProps<{
    open: boolean;
    title?: string;
    message?: string;
    confirmText?: string;
    cancelText?: string;
    danger?: boolean;
    loading?: boolean;
  }>(),
  { confirmText: '确认', cancelText: '取消' },
);

const emit = defineEmits<{ confirm: []; cancel: [] }>();
</script>

<template>
  <!-- Dismissing by ESC or backdrop is a cancel, never a confirm: the two must
       not be confused on a destructive action. -->
  <AppModal :open="open" :title="title" size="sm" @close="emit('cancel')">
    <div class="flex items-start gap-3">
      <span
        v-if="danger"
        class="grid size-9 shrink-0 place-items-center rounded-full bg-rose-50 text-rose-600 dark:bg-rose-500/10 dark:text-rose-400"
        aria-hidden="true"
      >
        <AlertTriangle class="size-4" />
      </span>
      <div class="min-w-0 flex-1">
        <p v-if="message" class="text-sm text-muted text-balance">{{ message }}</p>
        <slot />
      </div>
    </div>

    <template #footer>
      <AppButton variant="secondary" :disabled="loading" @click="emit('cancel')">
        {{ cancelText }}
      </AppButton>
      <AppButton
        :variant="danger ? 'danger' : 'primary'"
        :loading="loading"
        @click="emit('confirm')"
      >
        {{ confirmText }}
      </AppButton>
    </template>
  </AppModal>
</template>
