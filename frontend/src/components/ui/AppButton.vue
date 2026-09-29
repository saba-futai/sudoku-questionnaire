<script setup lang="ts">
import { Loader2 } from 'lucide-vue-next';
import { computed } from 'vue';

type Variant = 'primary' | 'secondary' | 'ghost' | 'danger' | 'subtle';
type Size = 'sm' | 'md' | 'lg';

const props = withDefaults(
  defineProps<{
    variant?: Variant;
    size?: Size;
    loading?: boolean;
    disabled?: boolean;
    block?: boolean;
    type?: 'button' | 'submit' | 'reset';
  }>(),
  { variant: 'primary', size: 'md', type: 'button' },
);

const VARIANTS: Record<Variant, string> = {
  primary:
    'bg-brand-600 text-white shadow-sm hover:bg-brand-500 active:bg-brand-700 ' +
    'dark:bg-brand-500 dark:hover:bg-brand-400 dark:active:bg-brand-600',
  secondary:
    'bg-raised text-ink border border-line hover:border-line-strong hover:bg-sunken active:bg-sunken',
  ghost: 'text-muted hover:text-ink hover:bg-sunken',
  subtle: 'bg-sunken text-ink hover:bg-line',
  danger:
    'bg-rose-600 text-white shadow-sm hover:bg-rose-500 active:bg-rose-700 dark:bg-rose-500 dark:hover:bg-rose-400',
};

const SIZES: Record<Size, string> = {
  sm: 'h-8 px-3 text-[13px] gap-1.5 rounded-lg',
  md: 'h-10 px-4 text-sm gap-2 rounded-[var(--radius-control)]',
  lg: 'h-12 px-6 text-[15px] gap-2 rounded-xl',
};

const classes = computed(() => [
  'inline-flex items-center justify-center font-medium whitespace-nowrap',
  'transition-[background-color,border-color,color,box-shadow,transform] duration-200',
  'active:scale-[0.985] disabled:pointer-events-none disabled:opacity-45',
  VARIANTS[props.variant],
  SIZES[props.size],
  props.block ? 'w-full' : '',
]);

const isDisabled = computed(() => props.disabled || props.loading);
</script>

<template>
  <button :type="type" :class="classes" :disabled="isDisabled" :aria-busy="loading || undefined">
    <Loader2 v-if="loading" class="size-4 animate-spin" />
    <slot />
  </button>
</template>
