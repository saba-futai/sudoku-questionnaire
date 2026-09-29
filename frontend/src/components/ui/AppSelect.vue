<script setup lang="ts">
import { ChevronDown } from 'lucide-vue-next';
import { computed, useId } from 'vue';

export interface SelectOption {
  value: string;
  label: string;
}

const props = withDefaults(
  defineProps<{
    modelValue?: string;
    label?: string;
    help?: string;
    error?: string;
    required?: boolean;
    disabled?: boolean;
    placeholder?: string;
    options?: SelectOption[];
  }>(),
  { options: () => [] },
);

const emit = defineEmits<{ 'update:modelValue': [string] }>();

const id = useId();
const helpId = computed(() => `${id}-help`);

function onChange(event: Event) {
  emit('update:modelValue', (event.target as HTMLSelectElement).value);
}
</script>

<template>
  <div class="w-full">
    <label v-if="label" :for="id" class="mb-1.5 flex items-center gap-1.5 text-sm font-medium text-ink">
      {{ label }}
      <span v-if="required" class="text-rose-500" aria-hidden="true">*</span>
    </label>

    <div
      class="focus-ring relative flex items-center rounded-[var(--radius-control)] border bg-raised"
      :class="error ? 'border-rose-400 dark:border-rose-500/70' : 'border-line'"
    >
      <!-- appearance-none + an overlay chevron: the native arrow cannot be
           themed, and it would sit on top of the padding we need on mobile. -->
      <select
        :id="id"
        :value="modelValue"
        :disabled="disabled"
        :required="required"
        :aria-invalid="error ? 'true' : undefined"
        :aria-describedby="help || error ? helpId : undefined"
        class="h-11 w-full min-w-0 cursor-pointer appearance-none bg-transparent pl-3 pr-9 text-sm text-ink outline-none disabled:cursor-not-allowed sm:h-10"
        @change="onChange"
      >
        <option v-if="placeholder" value="" disabled>{{ placeholder }}</option>
        <option v-for="opt in options" :key="opt.value" :value="opt.value">{{ opt.label }}</option>
      </select>
      <ChevronDown
        class="pointer-events-none absolute right-3 size-4 text-subtle transition-colors"
        :class="disabled ? 'opacity-50' : ''"
        aria-hidden="true"
      />
    </div>

    <p v-if="error" :id="helpId" class="mt-1.5 text-xs text-rose-600 dark:text-rose-400">{{ error }}</p>
    <p v-else-if="help" :id="helpId" class="mt-1.5 text-xs text-subtle">{{ help }}</p>
  </div>
</template>
