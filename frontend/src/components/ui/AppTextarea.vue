<script setup lang="ts">
import { computed, useId } from 'vue';

const props = withDefaults(
  defineProps<{
    modelValue?: string;
    label?: string;
    help?: string;
    error?: string;
    placeholder?: string;
    required?: boolean;
    disabled?: boolean;
    rows?: number;
    maxlength?: number;
    showCount?: boolean;
  }>(),
  { rows: 5 },
);

const emit = defineEmits<{ 'update:modelValue': [string] }>();

const id = useId();
const helpId = computed(() => `${id}-help`);
const length = computed(() => (props.modelValue ?? '').length);
const overLimit = computed(() => props.maxlength !== undefined && length.value > props.maxlength);

function onInput(event: Event) {
  emit('update:modelValue', (event.target as HTMLTextAreaElement).value);
}
</script>

<template>
  <div class="w-full">
    <label v-if="label" :for="id" class="mb-1.5 flex items-center gap-1.5 text-sm font-medium text-ink">
      {{ label }}
      <span v-if="required" class="text-rose-500" aria-hidden="true">*</span>
    </label>

    <div
      class="focus-ring rounded-[var(--radius-control)] border bg-raised"
      :class="error || overLimit ? 'border-rose-400 dark:border-rose-500/70' : 'border-line'"
    >
      <textarea
        :id="id"
        :value="modelValue"
        :rows="rows"
        :placeholder="placeholder"
        :disabled="disabled"
        :required="required"
        :maxlength="maxlength"
        :aria-invalid="error ? 'true' : undefined"
        :aria-describedby="help || error ? helpId : undefined"
        class="block w-full resize-y bg-transparent px-3 py-2.5 text-sm text-ink outline-none placeholder:text-subtle disabled:cursor-not-allowed"
        @input="onInput"
      />
    </div>

    <div class="mt-1.5 flex items-start justify-between gap-3">
      <p v-if="error" :id="helpId" class="text-xs text-rose-600 dark:text-rose-400">{{ error }}</p>
      <p v-else-if="help" :id="helpId" class="text-xs text-subtle">{{ help }}</p>
      <span v-else />
      <span
        v-if="showCount || maxlength"
        class="ml-auto shrink-0 font-mono text-xs tabular-nums"
        :class="overLimit ? 'text-rose-500' : 'text-subtle'"
      >
        {{ length }}<template v-if="maxlength">/{{ maxlength }}</template>
      </span>
    </div>
  </div>
</template>
