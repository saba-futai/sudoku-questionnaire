<script setup lang="ts">
import {
  AlertTriangle,
  ArrowRight,
  Braces,
  ChevronRight,
  ClipboardList,
  CircleCheck,
  Database,
  Image as ImageIcon,
  Inbox,
  LayoutGrid,
  RefreshCw,
  Sparkles,
  TrendingUp,
} from 'lucide-vue-next';
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import { RouterLink } from 'vue-router';

import { api } from '@/api/client';
import type { LeaderboardEntry, OverviewCounts } from '@/api/types';
import AppButton from '@/components/ui/AppButton.vue';
import AppCard from '@/components/ui/AppCard.vue';
import AppSkeleton from '@/components/ui/AppSkeleton.vue';
import SudokuMark from '@/components/ui/SudokuMark.vue';
import { formatNumber } from '@/lib/format';
import { errorMessage, toastError } from '@/lib/toast';

interface OverviewPayload {
  counts: OverviewCounts;
  leaderboard: LeaderboardEntry[];
}

const data = ref<OverviewPayload | null>(null);
const loading = ref(true);
const error = ref('');

const counts = computed(() => data.value?.counts ?? null);
const leaderboard = computed(() => data.value?.leaderboard ?? []);
const leaderboardMax = computed(() =>
  leaderboard.value.reduce((max, entry) => Math.max(max, entry.count), 0),
);

/**
 * 0 → 1 driver for the counters. Animating one driver instead of one tween per
 * tile keeps every number landing on the same frame.
 */
const progress = ref(0);
let frame = 0;

function playCountUp(): void {
  window.cancelAnimationFrame(frame);
  if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) {
    progress.value = 1;
    return;
  }
  const started = performance.now();
  const DURATION = 780;
  const step = (now: number) => {
    const t = Math.min(1, (now - started) / DURATION);
    progress.value = 1 - Math.pow(1 - t, 3); // ease-out cubic
    if (t < 1) frame = window.requestAnimationFrame(step);
  };
  frame = window.requestAnimationFrame(step);
}

function display(value: number): string {
  return formatNumber(Math.round(value * progress.value));
}

interface Tile {
  label: string;
  value: number;
  icon: unknown;
  accent: string;
}

const tiles = computed<Tile[]>(() => {
  const c = counts.value;
  return [
    {
      label: '问卷总数',
      value: c?.questionnaires ?? 0,
      icon: ClipboardList,
      accent: 'bg-brand-50 text-brand-600 dark:bg-brand-500/12 dark:text-brand-300',
    },
    {
      label: '已发布',
      value: c?.published ?? 0,
      icon: CircleCheck,
      accent: 'bg-emerald-50 text-emerald-600 dark:bg-emerald-500/12 dark:text-emerald-300',
    },
    {
      label: '答卷总数',
      value: c?.responses ?? 0,
      icon: Inbox,
      accent: 'bg-sky-50 text-sky-600 dark:bg-sky-500/12 dark:text-sky-300',
    },
    {
      label: '今日提交',
      value: c?.responsesToday ?? 0,
      icon: TrendingUp,
      accent: 'bg-pencil-500/12 text-pencil-600 dark:bg-pencil-400/12 dark:text-pencil-300',
    },
    {
      label: '图片数',
      value: c?.images ?? 0,
      icon: ImageIcon,
      accent: 'bg-violet-50 text-violet-600 dark:bg-violet-500/12 dark:text-violet-300',
    },
  ];
});

async function load(): Promise<void> {
  loading.value = true;
  error.value = '';
  try {
    data.value = await api<OverviewPayload>('/api/admin/overview');
    playCountUp();
  } catch (err) {
    error.value = errorMessage(err);
    toastError(err, '无法加载概览');
  } finally {
    loading.value = false;
  }
}

onMounted(load);
onBeforeUnmount(() => window.cancelAnimationFrame(frame));
</script>

<template>
  <div class="space-y-5 sm:space-y-6">
    <!-- Failure -->
    <AppCard v-if="error && !loading" padding="lg">
      <div class="flex flex-col items-center py-6 text-center">
        <span class="grid size-12 place-items-center rounded-2xl bg-rose-50 text-rose-600 dark:bg-rose-500/12 dark:text-rose-300">
          <AlertTriangle class="size-6" />
        </span>
        <h2 class="mt-4 text-base font-semibold text-ink">概览加载失败</h2>
        <p class="mt-1.5 max-w-sm text-sm text-muted text-balance">{{ error }}</p>
        <AppButton class="mt-5" variant="secondary" @click="load">
          <RefreshCw class="size-4" />
          重试
        </AppButton>
      </div>
    </AppCard>

    <template v-else>
      <!-- Stat tiles. The entrance animation lives on the wrapper, not on the
           card itself: a filled animation would otherwise out-rank the hover
           transform. -->
      <div class="stagger grid grid-cols-2 gap-3 sm:gap-4 lg:grid-cols-5">
        <template v-if="loading">
          <div v-for="i in 5" :key="i" :style="{ '--i': i }">
            <div class="card h-full p-4">
              <div class="skeleton size-9 rounded-xl" />
              <div class="skeleton mt-4 h-7 w-16 rounded" />
              <div class="skeleton mt-2 h-3 w-14 rounded" />
            </div>
          </div>
        </template>
        <template v-else>
          <div v-for="(tile, i) in tiles" :key="tile.label" :style="{ '--i': i }">
            <div class="card card-hover h-full p-4 sm:p-5">
              <span class="grid size-9 place-items-center rounded-xl" :class="tile.accent">
                <component :is="tile.icon" class="size-[18px]" />
              </span>
              <p
                class="mt-4 text-2xl font-semibold tracking-tight tabular-nums text-ink sm:text-[28px]"
              >
                {{ display(tile.value) }}
              </p>
              <p class="mt-0.5 text-xs text-muted">{{ tile.label }}</p>
            </div>
          </div>
        </template>
      </div>

      <div class="grid gap-5 lg:grid-cols-3 lg:gap-6">
        <!-- Leaderboard -->
        <AppCard class="lg:col-span-2" padding="lg">
          <template #title>
            <h2 class="text-base font-semibold text-ink">最近提交最多的问卷</h2>
            <p class="mt-0.5 text-xs text-muted">按答卷数量排序,点击可直接查看答卷。</p>
          </template>

          <!-- Height set inline so it wins over the component's own sizing and
               the placeholder matches the rows it stands in for. -->
          <div v-if="loading" class="space-y-2">
            <AppSkeleton
              v-for="i in 4"
              :key="i"
              variant="rect"
              :style="{ height: '3.5rem' }"
            />
          </div>

          <p v-else-if="!leaderboard.length" class="py-10 text-center text-sm text-subtle">
            还没有收到任何答卷。
          </p>

          <ul v-else class="stagger space-y-1">
            <li v-for="(entry, i) in leaderboard" :key="entry.id" :style="{ '--i': i }">
              <RouterLink
                :to="`/admin/questionnaires/${entry.id}/responses`"
                class="group flex items-center gap-4 rounded-[var(--radius-control)] px-3 py-3 transition-colors hover:bg-sunken"
              >
                <span
                  class="grid size-8 shrink-0 place-items-center rounded-lg font-mono text-xs font-semibold"
                  :class="
                    i === 0
                      ? 'bg-pencil-400/20 text-pencil-600 dark:text-pencil-300'
                      : 'bg-sunken text-subtle'
                  "
                >
                  {{ i + 1 }}
                </span>

                <div class="min-w-0 flex-1">
                  <p class="truncate text-sm font-medium text-ink">{{ entry.title }}</p>
                  <p class="truncate font-mono text-[11px] text-subtle">/q/{{ entry.slug }}</p>
                  <!-- A slim share bar: enough to rank by eye without a chart. -->
                  <div class="mt-2 h-1 w-full overflow-hidden rounded-full bg-sunken">
                    <div
                      class="h-full rounded-full bg-brand-500/70 transition-[width] duration-700 ease-out"
                      :style="{
                        width: leaderboardMax ? `${(entry.count / leaderboardMax) * 100}%` : '0%',
                      }"
                    />
                  </div>
                </div>

                <span class="shrink-0 text-sm font-semibold tabular-nums text-ink">
                  {{ formatNumber(entry.count) }}
                </span>
                <ChevronRight
                  class="size-4 shrink-0 text-subtle transition group-hover:translate-x-0.5 group-hover:text-muted"
                />
              </RouterLink>
            </li>
          </ul>
        </AppCard>

        <!-- Pipeline explainer -->
        <AppCard padding="lg" class="flex flex-col">
          <div class="flex items-center gap-3">
            <SudokuMark size="sm" />
            <div>
              <h2 class="text-base font-semibold text-ink">数独编码管线</h2>
              <p class="text-xs text-subtle">数据在写入前就已加密</p>
            </div>
          </div>

          <ol class="mt-5 space-y-3">
            <li class="flex items-start gap-3">
              <span class="grid size-7 shrink-0 place-items-center rounded-lg bg-brand-50 text-brand-600 dark:bg-brand-500/12 dark:text-brand-300">
                <Braces class="size-3.5" />
              </span>
              <div>
                <p class="text-sm font-medium text-ink">明文</p>
                <p class="text-xs text-muted">管理员始终看到解码后的内容</p>
              </div>
            </li>
            <li class="flex items-start gap-3">
              <span class="grid size-7 shrink-0 place-items-center rounded-lg bg-pencil-500/12 text-pencil-600 dark:text-pencil-300">
                <LayoutGrid class="size-3.5" />
              </span>
              <div>
                <p class="text-sm font-medium text-ink">数独编码</p>
                <p class="text-xs text-muted">字节映射为 4×4 合法终盘</p>
              </div>
            </li>
            <li class="flex items-start gap-3">
              <span class="grid size-7 shrink-0 place-items-center rounded-lg bg-sunken text-muted">
                <Database class="size-3.5" />
              </span>
              <div>
                <p class="text-sm font-medium text-ink">存储</p>
                <p class="text-xs text-muted">数据库里只有密文,没有明文</p>
              </div>
            </li>
          </ol>

          <RouterLink
            to="/admin/codec"
            class="group mt-5 flex items-center justify-between rounded-[var(--radius-control)] border border-line bg-sunken/60 px-3.5 py-3 text-sm font-medium text-ink transition hover:border-line-strong"
          >
            <span class="flex items-center gap-2">
              <Sparkles class="size-4 text-brand-500" />
              打开数独检视器
            </span>
            <ArrowRight class="size-4 text-subtle transition group-hover:translate-x-0.5" />
          </RouterLink>
        </AppCard>
      </div>
    </template>
  </div>
</template>
