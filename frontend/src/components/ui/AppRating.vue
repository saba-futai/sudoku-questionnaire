<script setup lang="ts">
import { Star, X } from 'lucide-vue-next';
import { computed, nextTick, ref } from 'vue';

const props = withDefaults(
  defineProps<{
    modelValue?: number | null;
    min?: number;
    max?: number;
    readonly?: boolean;
    size?: 'sm' | 'md' | 'lg';
  }>(),
  { modelValue: null, min: 1, max: 5, size: 'md' },
);

const emit = defineEmits<{ 'update:modelValue': [number | null] }>();

const root = ref<HTMLElement | null>(null);
const hovered = ref<number | null>(null);

const stars = computed(() =>
  Array.from({ length: Math.max(0, props.max - props.min + 1) }, (_, i) => props.min + i),
);
// While the pointer is over the row the preview wins over the stored value, so
// the user sees exactly what a click would commit.
const shown = computed(() => hovered.value ?? props.modelValue ?? 0);

const SIZES = {
  sm: { box: 'size-9', icon: 'size-4' },
  md: { box: 'size-11 sm:size-10', icon: 'size-6' },
  lg: { box: 'size-12', icon: 'size-8' },
} as const;

function pick(value: number) {
  if (props.readonly) return;
  hovered.value = null;
  emit('update:modelValue', value);
}

function move(delta: number) {
  const current = props.modelValue;
  if (current === null || current === undefined) {
    set(props.min);
    return;
  }
  set(Math.min(props.max, Math.max(props.min, current + delta)));
}

function set(value: number) {
  emit('update:modelValue', value);
  // The stars are a single roving tab stop, so keyboard focus has to follow the
  // selection or the next arrow press would start from the wrong place.
  nextTick(() => root.value?.querySelector<HTMLButtonElement>(`[data-star="${value}"]`)?.focus());
}

function onKeydown(event: KeyboardEvent) {
  if (props.readonly) return;
  const KEYS: Record<string, () => void> = {
    ArrowRight: () => move(1),
    ArrowUp: () => move(1),
    ArrowLeft: () => move(-1),
    ArrowDown: () => move(-1),
    Home: () => set(props.min),
    End: () => set(props.max),
  };
  const action = KEYS[event.key];
  if (!action) return;
  event.preventDefault();
  action();
}

function preview(value: number) {
  if (!props.readonly) hovered.value = value;
}

function clear() {
  hovered.value = null;
  emit('update:modelValue', null);
}
</script>

<template>
  <div
    ref="root"
    class="inline-flex items-center gap-0.5"
    :role="readonly ? 'img' : 'radiogroup'"
    :aria-label="readonly ? `${modelValue ?? 0} 分，满分 ${max} 分` : '评分'"
    @keydown="onKeydown"
    @mouseleave="hovered = null"
  >
    <button
      v-for="s in stars"
      :key="s"
      type="button"
      :data-star="s"
      :disabled="readonly"
      :tabindex="readonly ? -1 : (modelValue ?? min) === s ? 0 : -1"
      role="radio"
      :aria-checked="modelValue === s"
      :aria-label="`${s} 分`"
      class="grid shrink-0 place-items-center rounded-full transition-transform duration-200 disabled:cursor-default"
      :class="[
        SIZES[size].box,
        readonly ? '' : 'hover:scale-110 active:scale-95',
        s <= shown ? 'text-pencil-500' : 'text-subtle',
      ]"
      @click="pick(s)"
      @mouseenter="preview(s)"
      @focus="preview(s)"
      @blur="hovered = null"
    >
      <Star
        :class="SIZES[size].icon"
        :fill="s <= shown ? 'currentColor' : 'none'"
        :stroke-width="s <= shown ? 1 : 1.6"
        aria-hidden="true"
      />
    </button>

    <button
      v-if="!readonly && modelValue !== null && modelValue !== undefined"
      type="button"
      class="ml-1 grid size-11 shrink-0 place-items-center rounded-full text-subtle transition-colors hover:bg-sunken hover:text-ink sm:size-9"
      aria-label="清除评分"
      @click="clear"
    >
      <X class="size-4" aria-hidden="true" />
    </button>
  </div>
</template>
