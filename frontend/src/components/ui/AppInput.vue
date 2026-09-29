<script setup lang="ts">
import { computed, useId } from 'vue';

const props = withDefaults(
  defineProps<{
    modelValue?: string | number;
    label?: string;
    help?: string;
    error?: string;
    placeholder?: string;
    type?: string;
    required?: boolean;
    disabled?: boolean;
    prefix?: string;
    maxlength?: number;
    autocomplete?: string;
    inputmode?: 'text' | 'numeric' | 'decimal' | 'email' | 'tel' | 'url' | 'search';
  }>(),
  { type: 'text' },
);

const emit = defineEmits<{ 'update:modelValue': [string] }>();

const id = useId();
const helpId = computed(() => `${id}-help`);

function onInput(event: Event) {
  emit('update:modelValue', (event.target as HTMLInputElement).value);
}
</script>

<template>
  <div class="w-full">
    <label v-if="label" :for="id" class="mb-1.5 flex items-center gap-1.5 text-sm font-medium text-ink">
      {{ label }}
      <span v-if="required" class="text-rose-500" aria-hidden="true">*</span>
    </label>

    <div
      class="focus-ring flex items-center gap-2 rounded-[var(--radius-control)] border bg-raised px-3"
      :class="error ? 'border-rose-400 dark:border-rose-500/70' : 'border-line'"
    >
      <span v-if="prefix" class="shrink-0 text-sm text-subtle">{{ prefix }}</span>
      <input
        :id="id"
        :type="type"
        :value="modelValue"
        :placeholder="placeholder"
        :disabled="disabled"
        :required="required"
        :maxlength="maxlength"
        :autocomplete="autocomplete"
        :inputmode="inputmode"
        :aria-invalid="error ? 'true' : undefined"
        :aria-describedby="help || error ? helpId : undefined"
        class="h-10 w-full min-w-0 bg-transparent text-sm text-ink outline-none placeholder:text-subtle disabled:cursor-not-allowed"
        @input="onInput"
      />
    </div>

    <p v-if="error" :id="helpId" class="mt-1.5 text-xs text-rose-600 dark:text-rose-400">{{ error }}</p>
    <p v-else-if="help" :id="helpId" class="mt-1.5 text-xs text-subtle">{{ help }}</p>
  </div>
</template>
