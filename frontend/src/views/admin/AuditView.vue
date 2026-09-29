<script setup lang="ts">
import {
  AlertTriangle,
  ClipboardList,
  Inbox,
  KeyRound,
  Lock,
  Pencil,
  RefreshCw,
  ScrollText,
  ShieldAlert,
  Trash2,
} from 'lucide-vue-next';
import { computed, onMounted, ref } from 'vue';

import { api } from '@/api/client';
import type { AuditLog } from '@/api/types';
import AppButton from '@/components/ui/AppButton.vue';
import AppCard from '@/components/ui/AppCard.vue';
import EmptyState from '@/components/ui/EmptyState.vue';
import { formatDate, relativeTime } from '@/lib/format';
import { errorMessage, toastError } from '@/lib/toast';

const items = ref<AuditLog[]>([]);
const loading = ref(true);
const error = ref('');

/** Auditing decisions are made by a human reading this list, so every action
 *  the backend can record needs a sentence, not a dotted identifier. */
const ACTION_LABELS: Record<string, string> = {
  'questionnaire.create': '创建问卷',
  'questionnaire.update': '更新问卷',
  'questionnaire.delete': '删除问卷',
  'questionnaire.status': '变更状态',
  'questionnaire.export': '导出数据',
  'response.update': '修改答卷',
  'response.delete': '删除答卷',
  'login.success': '登录成功',
  'login.failed': '登录失败',
  'password.change': '修改密码',
};

const ACTION_ICONS: Record<string, unknown> = {
  'questionnaire.create': ClipboardList,
  'questionnaire.update': Pencil,
  'questionnaire.delete': Trash2,
  'questionnaire.status': ClipboardList,
  'questionnaire.export': ScrollText,
  'response.update': Inbox,
  'response.delete': Trash2,
  'login.success': KeyRound,
  'login.failed': ShieldAlert,
  'password.change': Lock,
};

/** Falls back to the action's family so an unmapped action still gets an icon. */
const FAMILY_ICONS: Record<string, unknown> = {
  questionnaire: ClipboardList,
  response: Inbox,
  login: KeyRound,
  password: Lock,
};

interface Entry {
  log: AuditLog;
  label: string;
  icon: unknown;
  failed: boolean;
}

const entries = computed<Entry[]>(() =>
  items.value.map((log) => {
    const family = log.action.split('.')[0] ?? '';
    return {
      log,
      label: ACTION_LABELS[log.action] ?? log.action,
      icon: ACTION_ICONS[log.action] ?? FAMILY_ICONS[family] ?? ScrollText,
      failed: log.action === 'login.failed',
    };
  }),
);

function actorLabel(actor: string): string {
  return actor?.trim() ? actor : '未知来源';
}

/** Turns the raw detail column into something an operator can read. */
function detailLabel(log: AuditLog): string {
  if (!log.detail) return '';
  if (log.action === 'questionnaire.status') {
    const map: Record<string, string> = { draft: '草稿', published: '已发布', closed: '已关闭' };
    return map[log.detail] ?? log.detail;
  }
  return log.detail;
}

function targetLabel(log: AuditLog): string {
  return log.targetId ? `${log.targetType} #${log.targetId}` : log.targetType;
}

async function load(): Promise<void> {
  loading.value = true;
  error.value = '';
  try {
    const payload = await api<{ items: AuditLog[] }>('/api/admin/audit?limit=100');
    items.value = payload.items ?? [];
  } catch (err) {
    error.value = errorMessage(err);
    toastError(err, '无法加载操作日志');
  } finally {
    loading.value = false;
  }
}

onMounted(load);
</script>

<template>
  <div class="space-y-5">
    <div class="flex items-center justify-between gap-3">
      <p class="text-sm text-muted">最近的 100 条管理操作,按时间倒序排列。</p>
      <AppButton variant="ghost" size="sm" :loading="loading" @click="load">
        <RefreshCw v-if="!loading" class="size-4" />
        刷新
      </AppButton>
    </div>

    <!-- Failure -->
    <AppCard v-if="error && !loading" padding="lg">
      <div class="flex flex-col items-center py-6 text-center">
        <span class="grid size-12 place-items-center rounded-2xl bg-rose-50 text-rose-600 dark:bg-rose-500/12 dark:text-rose-300">
          <AlertTriangle class="size-6" />
        </span>
        <h2 class="mt-4 text-base font-semibold text-ink">操作日志加载失败</h2>
        <p class="mt-1.5 max-w-sm text-sm text-muted text-balance">{{ error }}</p>
        <AppButton class="mt-5" variant="secondary" @click="load">
          <RefreshCw class="size-4" />
          重试
        </AppButton>
      </div>
    </AppCard>

    <AppCard v-else-if="loading" padding="lg">
      <div class="space-y-5">
        <div v-for="i in 8" :key="i" class="flex gap-4">
          <div class="skeleton size-9 shrink-0 rounded-full" />
          <div class="flex-1 space-y-2">
            <div class="skeleton h-4 w-1/3 rounded" />
            <div class="skeleton h-3 w-1/2 rounded" />
          </div>
        </div>
      </div>
    </AppCard>

    <AppCard v-else-if="!entries.length" padding="none">
      <EmptyState
        :icon="ScrollText"
        title="暂无操作日志"
        description="发布问卷、修改答卷等管理动作都会记录在这里。"
      />
    </AppCard>

    <!-- Timeline -->
    <AppCard v-else padding="lg">
      <ol class="stagger relative space-y-1">
        <li
          v-for="(entry, i) in entries"
          :key="entry.log.id"
          class="relative flex gap-4 pb-1"
          :style="{ '--i': Math.min(i, 12) }"
        >
          <!-- The rail, skipped on the last row so it does not trail off. -->
          <span
            v-if="i < entries.length - 1"
            class="absolute left-[17px] top-10 bottom-0 w-px bg-line"
            aria-hidden="true"
          />

          <span
            class="z-10 grid size-9 shrink-0 place-items-center rounded-full ring-4 ring-raised"
            :class="
              entry.failed
                ? 'bg-rose-100 text-rose-600 dark:bg-rose-500/15 dark:text-rose-300'
                : 'bg-sunken text-muted'
            "
          >
            <component :is="entry.icon" class="size-4" />
          </span>

          <div
            class="min-w-0 flex-1 rounded-[var(--radius-control)] border px-4 py-3 transition-colors"
            :class="
              entry.failed
                ? 'border-rose-200 bg-rose-50/70 dark:border-rose-400/25 dark:bg-rose-500/8'
                : 'border-line bg-raised hover:border-line-strong'
            "
          >
            <div class="flex flex-wrap items-center gap-x-2 gap-y-1">
              <p
                class="text-sm font-medium"
                :class="entry.failed ? 'text-rose-800 dark:text-rose-200' : 'text-ink'"
              >
                {{ entry.label }}
              </p>
              <span
                class="rounded-full bg-sunken px-2 py-0.5 font-mono text-[10px] text-subtle"
              >
                {{ entry.log.action }}
              </span>
            </div>

            <p class="mt-1 text-xs text-muted">
              <span class="font-medium text-ink">{{ actorLabel(entry.log.actor) }}</span>
              ·
              <span class="font-mono text-[11px]">{{ targetLabel(entry.log) }}</span>
              <template v-if="detailLabel(entry.log)">
                ·
                <span>{{ detailLabel(entry.log) }}</span>
              </template>
            </p>

            <p class="mt-1.5 text-[11px] text-subtle">
              <span class="tabular-nums">{{ formatDate(entry.log.createdAt) }}</span>
              <span class="mx-1.5">·</span>
              <span>{{ relativeTime(entry.log.createdAt) }}</span>
            </p>
          </div>
        </li>
      </ol>
    </AppCard>
  </div>
</template>
