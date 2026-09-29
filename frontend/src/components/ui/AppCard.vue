<script setup lang="ts">
withDefaults(
  defineProps<{
    padding?: 'none' | 'sm' | 'md' | 'lg';
    hover?: boolean;
    as?: string;
  }>(),
  { padding: 'md', hover: false, as: 'div' },
);

const PADDING = {
  none: '',
  sm: 'p-3 sm:p-4',
  md: 'p-4 sm:p-5',
  lg: 'p-5 sm:p-7',
} as const;
</script>

<template>
  <component :is="as" class="card" :class="[PADDING[padding], hover ? 'card-hover' : '']">
    <header v-if="$slots.header || $slots.title" class="mb-4 flex items-start justify-between gap-4">
      <div class="min-w-0">
        <slot name="title" />
        <slot name="header" />
      </div>
      <slot name="actions" />
    </header>
    <slot />
    <footer v-if="$slots.footer" class="mt-4 border-t border-line pt-4">
      <slot name="footer" />
    </footer>
  </component>
</template>
