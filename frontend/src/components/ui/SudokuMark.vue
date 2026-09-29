<script setup lang="ts">
/**
 * The product's signature mark: a 4x4 sudoku whose cells fill in sequence.
 *
 * It is not decoration for its own sake — 4x4 is the exact grid the codec
 * enumerates, so the mark doubles as a miniature of what the app does.
 */
import { computed, onMounted, ref } from 'vue';

const props = withDefaults(
  defineProps<{
    /** A solved 4x4 grid, row-major, values 1..4. Defaults to a canonical one. */
    grid?: number[];
    size?: 'sm' | 'md' | 'lg';
    animate?: boolean;
    /** Renders values as faint "pencil marks" rather than solid digits. */
    pencil?: boolean;
  }>(),
  {
    grid: () => [1, 2, 3, 4, 3, 4, 1, 2, 2, 1, 4, 3, 4, 3, 2, 1],
    size: 'md',
    animate: true,
    pencil: false,
  },
);

const revealed = ref(props.animate ? 0 : 16);

onMounted(() => {
  if (!props.animate) return;
  const timer = window.setInterval(() => {
    revealed.value += 1;
    if (revealed.value >= 16) window.clearInterval(timer);
  }, 55);
});

const CELL_SIZE = {
  sm: 'size-5 text-[10px] rounded-[3px]',
  md: 'size-8 text-sm rounded',
  lg: 'size-11 text-lg rounded-md',
} as const;

const cells = computed(() =>
  Array.from({ length: 16 }, (_, i) => {
    const row = Math.floor(i / 4);
    const col = i % 4;
    return {
      value: props.grid[i] ?? 0,
      boxRight: col === 1 ? '1' : undefined,
      boxBottom: row === 1 ? '1' : undefined,
      shown: i < revealed.value,
    };
  }),
);
</script>

<template>
  <div class="sudoku-grid" :style="{ gridTemplateColumns: 'repeat(4, min-content)' }" aria-hidden="true">
    <div
      v-for="(cell, i) in cells"
      :key="i"
      class="sudoku-cell"
      :class="[CELL_SIZE[size], cell.shown ? (pencil ? 'text-brand-400/80' : 'text-ink') : 'opacity-30']"
      :data-box-right="cell.boxRight"
      :data-box-bottom="cell.boxBottom"
      :style="{ animationDelay: `${i * 55}ms` }"
    >
      <span :class="cell.shown ? 'animate-cell-pop' : ''">{{ cell.shown ? cell.value : '·' }}</span>
    </div>
  </div>
</template>
