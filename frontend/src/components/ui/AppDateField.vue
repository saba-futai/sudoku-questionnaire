<script setup lang="ts">
import { computed, useId } from 'vue';

defineProps<{
  modelValue?: string;
  label?: string;
  help?: string;
  error?: string;
  required?: boolean;
  disabled?: boolean;
  min?: string;
  max?: string;
}>();

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
      class="focus-ring flex items-center rounded-[var(--radius-control)] border bg-raised px-3"
      :class="error ? 'border-rose-400 dark:border-rose-500/70' : 'border-line'"
    >
      <!-- The native picker is worth keeping — it is the only date UI that feels
           right on a phone — but its indicator must be told which theme it is in
           or it renders black-on-black. -->
      <input
        :id="id"
        type="date"
        :value="modelValue"
        :min="min"
        :max="max"
        :disabled="disabled"
        :required="required"
        :aria-invalid="error ? 'true' : undefined"
        :aria-describedby="help || error ? helpId : undefined"
        class="h-11 w-full min-w-0 bg-transparent text-sm text-ink outline-none [color-scheme:light] disabled:cursor-not-allowed disabled:opacity-60 sm:h-10 dark:[color-scheme:dark]"
        @input="onInput"
      />
    </div>

    <p v-if="error" :id="helpId" class="mt-1.5 text-xs text-rose-600 dark:text-rose-400">{{ error }}</p>
    <p v-else-if="help" :id="helpId" class="mt-1.5 text-xs text-subtle">{{ help }}</p>
  </div>
</template>
