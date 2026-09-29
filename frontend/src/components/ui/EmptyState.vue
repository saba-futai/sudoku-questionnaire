<script setup lang="ts">
import { Inbox } from 'lucide-vue-next';

withDefaults(
  defineProps<{
    title: string;
    description?: string;
    icon?: unknown;
  }>(),
  {},
);
</script>

<template>
  <div class="flex flex-col items-center justify-center px-6 py-14 text-center">
    <div class="mb-4 grid size-14 place-items-center rounded-2xl bg-sunken text-subtle">
      <component :is="icon ?? Inbox" class="size-6" />
    </div>
    <h3 class="text-base font-semibold text-ink">{{ title }}</h3>
    <p v-if="description" class="mt-1.5 max-w-sm text-sm text-muted text-balance">{{ description }}</p>
    <div v-if="$slots.default" class="mt-5">
      <slot />
    </div>
  </div>
</template>
