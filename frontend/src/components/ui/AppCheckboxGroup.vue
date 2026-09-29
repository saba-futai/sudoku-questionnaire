<script setup lang="ts">
import { Check } from 'lucide-vue-next';

export interface CheckboxOption {
  value: string;
  label: string;
  description?: string;
}

const props = withDefaults(
  defineProps<{
    modelValue?: string[];
    options?: CheckboxOption[];
    name?: string;
    columns?: 1 | 2 | 3;
    disabled?: boolean;
  }>(),
  { options: () => [], columns: 1 },
);

const emit = defineEmits<{ 'update:modelValue': [string[]] }>();

const FOCUS =
  'has-[:focus-visible]:border-[var(--ring)] has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-brand-500/35';

const COLUMNS = {
  1: '',
  2: 'sm:grid-cols-2',
  3: 'sm:grid-cols-3',
} as const;

const selected = () => props.modelValue ?? [];

function toggle(value: string) {
  const current = selected();
  emit(
    'update:modelValue',
    current.includes(value) ? current.filter((v) => v !== value) : [...current, value],
  );
}
</script>

<template>
  <div class="grid grid-cols-1 gap-2.5" :class="COLUMNS[columns]" role="group">
    <label
      v-for="opt in options"
      :key="opt.value"
      class="relative flex min-h-11 cursor-pointer items-start gap-3 rounded-[var(--radius-control)] border p-3.5 transition-colors duration-200"
      :class="[
        FOCUS,
        disabled ? 'cursor-not-allowed opacity-55' : '',
        selected().includes(opt.value)
          ? 'border-brand-500 bg-brand-50/70 ring-1 ring-brand-500 dark:border-brand-400 dark:bg-brand-500/12 dark:ring-brand-400'
          : 'border-line bg-raised hover:border-line-strong hover:bg-sunken',
      ]"
    >
      <input
        type="checkbox"
        class="sr-only"
        :name="name"
        :value="opt.value"
        :checked="selected().includes(opt.value)"
        :disabled="disabled"
        @change="toggle(opt.value)"
      />

      <span
        class="mt-0.5 grid size-5 shrink-0 place-items-center rounded-md border-2 transition-colors duration-200"
        :class="
          selected().includes(opt.value)
            ? 'border-brand-600 bg-brand-600 text-white dark:border-brand-400 dark:bg-brand-400 dark:text-brand-950'
            : 'border-line-strong'
        "
        aria-hidden="true"
      >
        <Check v-if="selected().includes(opt.value)" class="size-3.5 animate-cell-pop" :stroke-width="3" />
      </span>

      <span class="min-w-0">
        <span class="block text-sm font-medium text-ink text-balance">{{ opt.label }}</span>
        <span v-if="opt.description" class="mt-0.5 block text-xs text-subtle text-balance">
          {{ opt.description }}
        </span>
      </span>
    </label>
  </div>
</template>
