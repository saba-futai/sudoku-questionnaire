<script setup lang="ts">
import { Moon, Sun } from 'lucide-vue-next';
import { computed } from 'vue';

import { theme, toggleTheme } from '@/lib/theme';

// The stored ref is the single source of truth for the whole app, so the icon
// here can never disagree with the class on <html>.
const isDark = computed(() => theme.value === 'dark');
const label = computed(() => (isDark.value ? '切换到浅色模式' : '切换到深色模式'));
</script>

<template>
  <button
    type="button"
    class="relative grid size-11 shrink-0 place-items-center overflow-hidden rounded-[var(--radius-control)] text-muted transition-colors duration-200 hover:bg-sunken hover:text-ink sm:size-10"
    :aria-label="label"
    :title="label"
    :aria-pressed="isDark"
    @click="toggleTheme()"
  >
    <Transition name="theme-icon">
      <Sun v-if="isDark" key="sun" class="size-5" aria-hidden="true" />
      <Moon v-else key="moon" class="size-5" aria-hidden="true" />
    </Transition>
  </button>
</template>

<style scoped>
.theme-icon-enter-active,
.theme-icon-leave-active {
  transition:
    opacity 0.22s ease,
    transform 0.32s var(--ease-spring);
}
.theme-icon-enter-from {
  opacity: 0;
  transform: rotate(-70deg) scale(0.6);
}
.theme-icon-leave-to {
  opacity: 0;
  transform: rotate(70deg) scale(0.6);
}
/* Both icons share the button, so the outgoing one is taken out of flow for
   the duration of the swap instead of pushing the incoming one aside. */
.theme-icon-leave-active {
  position: absolute;
}
</style>
