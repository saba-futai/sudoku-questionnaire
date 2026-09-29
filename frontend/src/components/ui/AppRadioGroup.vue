<script setup lang="ts">
import { useId } from 'vue';

export interface RadioOption {
  value: string;
  label: string;
  description?: string;
}

const props = withDefaults(
  defineProps<{
    modelValue?: string;
    options?: RadioOption[];
    name?: string;
    columns?: 1 | 2 | 3;
    disabled?: boolean;
  }>(),
  { options: () => [], columns: 1 },
);

const emit = defineEmits<{ 'update:modelValue': [string] }>();

// Radios need a shared name to form a single arrow-key group; falling back to
// a per-instance id keeps two groups on one page from fighting each other.
const name = props.name ?? `radio-${useId()}`;

// The focused look lives on the label rather than an inner element so the
// target and the focus ring are the same 44px box.
const FOCUS =
  'has-[:focus-visible]:border-[var(--ring)] has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-brand-500/35';

const COLUMNS = {
  1: '',
  2: 'sm:grid-cols-2',
  3: 'sm:grid-cols-3',
} as const;
</script>

<template>
  <div
    class="grid grid-cols-1 gap-2.5"
    :class="COLUMNS[columns]"
    role="radiogroup"
    :aria-disabled="disabled || undefined"
  >
    <label
      v-for="opt in options"
      :key="opt.value"
      class="relative flex min-h-11 cursor-pointer items-start gap-3 rounded-[var(--radius-control)] border p-3.5 transition-colors duration-200"
      :class="[
        FOCUS,
        disabled ? 'cursor-not-allowed opacity-55' : '',
        modelValue === opt.value
          ? 'border-brand-500 bg-brand-50/70 ring-1 ring-brand-500 dark:border-brand-400 dark:bg-brand-500/12 dark:ring-brand-400'
          : 'border-line bg-raised hover:border-line-strong hover:bg-sunken',
      ]"
    >
      <input
        type="radio"
        class="sr-only"
        :name="name"
        :value="opt.value"
        :checked="modelValue === opt.value"
        :disabled="disabled"
        @change="emit('update:modelValue', opt.value)"
      />

      <span
        class="mt-0.5 grid size-5 shrink-0 place-items-center rounded-full border-2 transition-colors duration-200"
        :class="modelValue === opt.value ? 'border-brand-600 dark:border-brand-400' : 'border-line-strong'"
        aria-hidden="true"
      >
        <span
          v-if="modelValue === opt.value"
          class="size-2.5 rounded-full bg-brand-600 animate-cell-pop dark:bg-brand-400"
        />
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
