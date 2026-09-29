<script setup lang="ts">
import { X } from 'lucide-vue-next';
import { computed, nextTick, onBeforeUnmount, ref, useId, useSlots, watch } from 'vue';

const props = withDefaults(
  defineProps<{
    open: boolean;
    title?: string;
    description?: string;
    size?: 'sm' | 'md' | 'lg';
    closeOnBackdrop?: boolean;
  }>(),
  { size: 'md', closeOnBackdrop: true },
);

const emit = defineEmits<{ close: [] }>();

const slots = useSlots();
const id = useId();
const titleId = computed(() => `${id}-title`);
const descriptionId = computed(() => `${id}-description`);

const panel = ref<HTMLElement | null>(null);
let restoreFocus: HTMLElement | null = null;
let savedOverflow = '';

const SIZES = {
  sm: 'sm:max-w-sm',
  md: 'sm:max-w-lg',
  lg: 'sm:max-w-2xl',
} as const;

const FOCUSABLE =
  'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

function focusables(): HTMLElement[] {
  if (!panel.value) return [];
  return Array.from(panel.value.querySelectorAll<HTMLElement>(FOCUSABLE)).filter(
    (el) => el.offsetParent !== null,
  );
}

function onKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') {
    event.preventDefault();
    emit('close');
    return;
  }
  if (event.key !== 'Tab') return;

  const items = focusables();
  if (!items.length) {
    event.preventDefault();
    panel.value?.focus();
    return;
  }
  const first = items[0];
  const last = items[items.length - 1];
  const active = document.activeElement as HTMLElement | null;
  const inside = active !== null && panel.value?.contains(active);

  // Wrap at both ends so Tab can never reach the page behind the dialog.
  if (event.shiftKey && (active === first || !inside)) {
    event.preventDefault();
    last.focus();
  } else if (!event.shiftKey && (active === last || !inside)) {
    event.preventDefault();
    first.focus();
  }
}

watch(
  () => props.open,
  async (open) => {
    if (open) {
      restoreFocus = document.activeElement as HTMLElement | null;
      savedOverflow = document.body.style.overflow;
      document.body.style.overflow = 'hidden';
      // Capture phase, so ESC still closes the dialog when focus sits on a
      // control that swallows keydown (the date picker, for one).
      window.addEventListener('keydown', onKeydown, true);
      await nextTick();
      (focusables()[0] ?? panel.value)?.focus();
    } else if (restoreFocus) {
      window.removeEventListener('keydown', onKeydown, true);
      document.body.style.overflow = savedOverflow;
      // Focus goes back to whatever opened the dialog, so a keyboard user does
      // not land back at the top of the page.
      restoreFocus.focus();
      restoreFocus = null;
    }
  },
  { immediate: true },
);

onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKeydown, true);
  if (restoreFocus) document.body.style.overflow = savedOverflow;
});
</script>

<template>
  <Teleport to="body">
    <Transition name="modal">
      <div
        v-if="open"
        class="fixed inset-0 z-50 flex items-end justify-center sm:items-center sm:p-4"
        role="presentation"
      >
        <div
          class="absolute inset-0 bg-slate-950/50 backdrop-blur-sm"
          @click="closeOnBackdrop && emit('close')"
        />

        <div
          ref="panel"
          class="modal-panel card relative flex max-h-[92dvh] w-full flex-col outline-none rounded-b-none rounded-t-[var(--radius-card)] sm:max-h-[85dvh] sm:rounded-[var(--radius-card)]"
          :class="SIZES[size]"
          role="dialog"
          aria-modal="true"
          :aria-labelledby="title ? titleId : undefined"
          :aria-describedby="description ? descriptionId : undefined"
          tabindex="-1"
        >
          <header class="flex items-start justify-between gap-4 border-b border-line px-5 py-4">
            <div class="min-w-0">
              <h2 v-if="title" :id="titleId" class="text-base font-semibold text-ink text-balance">
                {{ title }}
              </h2>
              <p v-if="description" :id="descriptionId" class="mt-1 text-sm text-muted text-balance">
                {{ description }}
              </p>
              <slot name="title" />
            </div>
            <button
              type="button"
              class="-mr-1 -mt-1 grid size-9 shrink-0 place-items-center rounded-lg text-subtle transition-colors duration-200 hover:bg-sunken hover:text-ink"
              aria-label="关闭"
              @click="emit('close')"
            >
              <X class="size-5" aria-hidden="true" />
            </button>
          </header>

          <div class="min-h-0 flex-1 overflow-y-auto px-5 py-4">
            <slot />
          </div>

          <footer
            v-if="slots.footer"
            class="flex flex-col-reverse gap-2 border-t border-line px-5 py-4 sm:flex-row sm:justify-end"
          >
            <slot name="footer" />
          </footer>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.modal-enter-active,
.modal-leave-active {
  transition: opacity 0.24s ease;
}
.modal-enter-active .modal-panel {
  transition:
    transform 0.36s var(--ease-spring),
    opacity 0.24s ease;
}
.modal-leave-active .modal-panel {
  transition:
    transform 0.18s ease-in,
    opacity 0.18s ease-in;
}
.modal-enter-from,
.modal-leave-to {
  opacity: 0;
}
.modal-enter-from .modal-panel {
  opacity: 0;
  transform: translateY(18px) scale(0.96);
}
.modal-leave-to .modal-panel {
  opacity: 0;
  transform: translateY(8px) scale(0.98);
}
</style>
