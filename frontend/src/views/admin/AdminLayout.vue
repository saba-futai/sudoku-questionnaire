<script setup lang="ts">
/**
 * The admin shell. Everything under /admin renders inside this: a permanent
 * sidebar on desktop, a drawer on phones, and one place that owns navigation,
 * the page title and the session controls.
 */
import {
  AlertTriangle,
  ChevronRight,
  ClipboardList,
  LayoutDashboard,
  LayoutGrid,
  LogOut,
  Menu,
  ScrollText,
  Settings,
  X,
} from 'lucide-vue-next';
import { computed, onBeforeUnmount, ref, watch } from 'vue';
import { RouterLink, RouterView, useRoute, useRouter } from 'vue-router';

import AppButton from '@/components/ui/AppButton.vue';
import SudokuMark from '@/components/ui/SudokuMark.vue';
import ThemeToggle from '@/components/ui/ThemeToggle.vue';
import { useAuthStore } from '@/stores/auth';
import { useCodecStore } from '@/stores/codec';

const route = useRoute();
const router = useRouter();
const auth = useAuthStore();
const codec = useCodecStore();

interface NavItem {
  to: string;
  label: string;
  icon: unknown;
}

const NAV: NavItem[] = [
  { to: '/admin', label: '概览', icon: LayoutDashboard },
  { to: '/admin/questionnaires', label: '问卷管理', icon: ClipboardList },
  { to: '/admin/codec', label: '数独检视器', icon: LayoutGrid },
  { to: '/admin/audit', label: '操作日志', icon: ScrollText },
  { to: '/admin/settings', label: '设置', icon: Settings },
];

const drawerOpen = ref(false);

const pageTitle = computed(() => (route.meta.title as string | undefined) || '管理后台');

/** True when the current route belongs to a nav entry. `/admin` is a prefix of
 *  every other entry, so it is the one that needs an exact match. */
function isActive(to: string): boolean {
  if (to === '/admin') return route.path === '/admin';
  return route.path === to || route.path.startsWith(`${to}/`);
}

function closeDrawer(): void {
  drawerOpen.value = false;
}

function onLogout(): void {
  auth.logout();
  closeDrawer();
  void router.replace({ name: 'admin-login' });
}

function onKeydown(event: KeyboardEvent): void {
  if (event.key === 'Escape') closeDrawer();
}

/** Drawer nav shares this: the drawer must close even when the target is the
 *  route already showing, where the path watcher would never fire. */
function go(navigate: (event?: MouseEvent) => void, event: MouseEvent): void {
  closeDrawer();
  navigate(event);
}

// Navigating from inside the drawer should dismiss it, otherwise the overlay
// stays on top of the page the user just asked for.
watch(() => route.fullPath, closeDrawer);

// Freeze the page behind the drawer so scrolling does not leak on touch.
watch(drawerOpen, (open) => {
  document.body.style.overflow = open ? 'hidden' : '';
  if (open) window.addEventListener('keydown', onKeydown);
  else window.removeEventListener('keydown', onKeydown);
});

onBeforeUnmount(() => {
  document.body.style.overflow = '';
  window.removeEventListener('keydown', onKeydown);
});
</script>

<template>
  <!-- Mobile top bar -->
  <header class="glass sticky top-0 z-30 border-b border-line lg:hidden">
    <div class="flex h-14 items-center gap-3 px-4">
      <button
        type="button"
        class="focus-ring -ml-1 grid size-9 place-items-center rounded-[var(--radius-control)] text-muted transition hover:bg-sunken hover:text-ink"
        aria-label="打开导航"
        :aria-expanded="drawerOpen"
        @click="drawerOpen = true"
      >
        <Menu class="size-5" />
      </button>

      <div class="flex min-w-0 flex-1 items-center gap-2.5">
        <SudokuMark size="sm" :animate="false" />
        <div class="min-w-0">
          <p class="truncate text-sm font-semibold tracking-tight text-ink">QRforQA</p>
          <p class="truncate text-[11px] text-subtle">{{ pageTitle }}</p>
        </div>
      </div>

      <RouterLink
        v-if="codec.mismatches.length"
        to="/admin/codec"
        class="grid size-9 place-items-center rounded-[var(--radius-control)] bg-rose-50 text-rose-600 transition hover:bg-rose-100 dark:bg-rose-500/10 dark:text-rose-400 dark:hover:bg-rose-500/20"
        :title="codec.mismatches[0]"
        aria-label="编码表不一致"
      >
        <AlertTriangle class="size-[18px]" />
      </RouterLink>
    </div>
  </header>

  <!-- Desktop sidebar -->
  <aside
    class="fixed inset-y-0 left-0 z-30 hidden w-[248px] flex-col border-r border-line bg-raised lg:flex"
  >
    <div class="flex items-center gap-3 px-5 py-5">
      <SudokuMark size="sm" />
      <div class="min-w-0">
        <p class="text-sm font-semibold tracking-tight text-ink">QRforQA</p>
        <p class="text-[11px] text-subtle">数独问卷系统</p>
      </div>
    </div>

    <nav class="flex-1 space-y-1 px-3 py-2" aria-label="后台导航">
      <RouterLink v-for="item in NAV" :key="item.to" :to="item.to" custom v-slot="{ href, navigate }">
        <a
          :href="href"
          class="group relative flex items-center gap-3 rounded-[var(--radius-control)] px-3 py-2.5 text-sm font-medium transition-colors duration-200"
          :class="
            isActive(item.to)
              ? 'bg-brand-50 text-brand-700 dark:bg-brand-500/12 dark:text-brand-200'
              : 'text-muted hover:bg-sunken hover:text-ink'
          "
          :aria-current="isActive(item.to) ? 'page' : undefined"
          @click="navigate"
        >
          <span
            v-if="isActive(item.to)"
            class="absolute inset-y-2 left-0 w-[3px] rounded-r-full bg-brand-500"
          />
          <component
            :is="item.icon"
            class="size-[18px] shrink-0 transition-colors"
            :class="
              isActive(item.to)
                ? 'text-brand-600 dark:text-brand-300'
                : 'text-subtle group-hover:text-muted'
            "
          />
          {{ item.label }}
        </a>
      </RouterLink>
    </nav>

    <div class="border-t border-line p-3">
      <div class="flex items-center gap-3 rounded-[var(--radius-control)] px-2 py-2">
        <span
          class="grid size-8 shrink-0 place-items-center rounded-full bg-brand-50 text-xs font-semibold text-brand-700 dark:bg-brand-500/12 dark:text-brand-200"
        >
          {{ (auth.username || '?').slice(0, 1).toUpperCase() }}
        </span>
        <div class="min-w-0 flex-1">
          <p class="truncate text-sm font-medium text-ink">{{ auth.username || '未登录' }}</p>
          <p class="text-[11px] text-subtle">管理员</p>
        </div>
        <ThemeToggle />
      </div>
      <AppButton variant="ghost" size="sm" block class="mt-1 justify-start" @click="onLogout">
        <LogOut class="size-4" />
        退出登录
      </AppButton>
    </div>
  </aside>

  <!-- Mobile drawer. One transition on the overlay; the panel slides in from its
       own keyframes so the two motions do not have to be nested. -->
  <Transition
    enter-active-class="transition-opacity duration-300 ease-out"
    enter-from-class="opacity-0"
    leave-active-class="transition-opacity duration-200 ease-in"
    leave-to-class="opacity-0"
  >
    <div v-if="drawerOpen" class="fixed inset-0 z-50 lg:hidden" role="dialog" aria-modal="true">
      <div class="absolute inset-0 bg-slate-900/45 backdrop-blur-[2px]" @click="closeDrawer" />

      <aside
        class="drawer-panel absolute inset-y-0 left-0 flex w-[82%] max-w-[304px] flex-col border-r border-line bg-raised shadow-2xl"
      >
        <div class="flex items-center gap-3 px-4 py-4">
          <SudokuMark size="sm" :animate="false" />
          <div class="min-w-0 flex-1">
            <p class="text-sm font-semibold tracking-tight text-ink">QRforQA</p>
            <p class="text-[11px] text-subtle">数独问卷系统</p>
          </div>
          <button
            type="button"
            class="grid size-9 place-items-center rounded-[var(--radius-control)] text-subtle transition hover:bg-sunken hover:text-ink"
            aria-label="关闭导航"
            @click="closeDrawer"
          >
            <X class="size-5" />
          </button>
        </div>

        <nav class="flex-1 space-y-1 overflow-y-auto px-3 py-2" aria-label="后台导航">
          <RouterLink
            v-for="item in NAV"
            :key="item.to"
            :to="item.to"
            custom
            v-slot="{ href, navigate }"
          >
            <a
              :href="href"
              class="group relative flex items-center gap-3 rounded-[var(--radius-control)] px-3 py-3 text-sm font-medium transition-colors"
              :class="
                isActive(item.to)
                  ? 'bg-brand-50 text-brand-700 dark:bg-brand-500/12 dark:text-brand-200'
                  : 'text-muted hover:bg-sunken hover:text-ink'
              "
              @click="go(navigate, $event)"
            >
              <span
                v-if="isActive(item.to)"
                class="absolute inset-y-2 left-0 w-[3px] rounded-r-full bg-brand-500"
              />
              <component :is="item.icon" class="size-[18px] shrink-0" />
              {{ item.label }}
            </a>
          </RouterLink>
        </nav>

        <div class="border-t border-line p-3">
          <p class="px-2 pb-2 text-xs text-subtle">
            已登录为 <span class="font-medium text-muted">{{ auth.username || '—' }}</span>
          </p>
          <AppButton variant="secondary" size="md" block @click="onLogout">
            <LogOut class="size-4" />
            退出登录
          </AppButton>
        </div>
      </aside>
    </div>
  </Transition>

  <!-- Content -->
  <div class="lg:pl-[248px]">
    <main class="mx-auto w-full max-w-[1200px] px-4 py-6 sm:px-6 sm:py-8 lg:px-10 lg:py-10">
      <!-- Keyed on the path so the whole block replays its entrance on nav. -->
      <div :key="route.path" class="animate-fade-in">
        <div class="mb-6 flex flex-wrap items-end justify-between gap-3 sm:mb-8">
          <div class="min-w-0">
            <nav class="flex items-center gap-1.5 text-xs text-subtle" aria-label="面包屑">
              <span>管理后台</span>
              <ChevronRight class="size-3.5 shrink-0 opacity-60" />
              <span class="truncate font-medium text-muted">{{ pageTitle }}</span>
            </nav>
            <h1 class="mt-1.5 text-2xl font-semibold tracking-tight text-ink sm:text-[28px]">
              {{ pageTitle }}
            </h1>
          </div>

          <!-- The browser and the server disagreeing on the coding table is the
               one failure the operator must never have to go looking for. -->
          <RouterLink
            v-if="codec.mismatches.length"
            to="/admin/codec"
            class="inline-flex items-center gap-2 rounded-full bg-rose-50 px-3 py-1.5 text-xs font-medium text-rose-700 ring-1 ring-rose-600/20 transition hover:bg-rose-100 dark:bg-rose-500/10 dark:text-rose-300 dark:ring-rose-400/25 dark:hover:bg-rose-500/20"
            :title="codec.mismatches[0]"
          >
            <AlertTriangle class="size-3.5" />
            编码表不一致 ({{ codec.mismatches.length }})
          </RouterLink>
        </div>

        <RouterView />
      </div>
    </main>
  </div>
</template>

<style scoped>
@keyframes drawer-in {
  from {
    transform: translateX(-100%);
  }
  to {
    transform: translateX(0);
  }
}

.drawer-panel {
  animation: drawer-in 0.34s var(--ease-out-soft) both;
}
</style>
