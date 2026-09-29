<script setup lang="ts">
import {
  AlertTriangle,
  CheckCircle2,
  Database,
  KeyRound,
  Monitor,
  Moon,
  RefreshCw,
  Server,
  Sun,
} from 'lucide-vue-next';
import { computed, onMounted, ref } from 'vue';

import { api } from '@/api/client';
import AppButton from '@/components/ui/AppButton.vue';
import AppCard from '@/components/ui/AppCard.vue';
import AppInput from '@/components/ui/AppInput.vue';
import { formatNumber } from '@/lib/format';
import { setThemePreference, themePreference, type ThemePreference } from '@/lib/theme';
import { errorMessage, toast, toastError } from '@/lib/toast';
import { useAuthStore } from '@/stores/auth';

/** Mirrors the shape of `GET /healthz` in backend/internal/handlers. */
interface HealthPayload {
  status: string;
  codec: string;
  layout: string;
  grids: number;
  positionSets: number;
  gridsSha256: string;
}

const auth = useAuthStore();

// ------------------------------------------------------------- password ---

const current = ref('');
const next = ref('');
const confirm = ref('');
const formError = ref('');
const saving = ref(false);

async function changePassword(): Promise<void> {
  if (saving.value) return;
  formError.value = '';

  if (!current.value) {
    formError.value = '请输入当前密码';
    return;
  }
  if (next.value.length < 8) {
    formError.value = '新密码至少需要 8 位';
    return;
  }
  if (next.value !== confirm.value) {
    formError.value = '两次输入的新密码不一致';
    return;
  }
  if (next.value === current.value) {
    formError.value = '新密码不能与当前密码相同';
    return;
  }

  saving.value = true;
  try {
    await auth.changePassword(current.value, next.value);
    toast.success('密码已更新', '下次登录请使用新密码');
    current.value = '';
    next.value = '';
    confirm.value = '';
  } catch (err) {
    formError.value = errorMessage(err);
    toastError(err, '修改密码失败');
  } finally {
    saving.value = false;
  }
}

// ------------------------------------------------------------ appearance ---

const THEMES: { value: ThemePreference; label: string; icon: unknown }[] = [
  { value: 'light', label: '浅色', icon: Sun },
  { value: 'dark', label: '深色', icon: Moon },
  { value: 'system', label: '跟随系统', icon: Monitor },
];

function chooseTheme(value: ThemePreference): void {
  setThemePreference(value);
}

// ---------------------------------------------------------------- system ---

const health = ref<HealthPayload | null>(null);
const healthLoading = ref(true);
const healthError = ref('');

const shortDigest = computed(() => {
  const hash = health.value?.gridsSha256 ?? '';
  return hash ? `${hash.slice(0, 16)}…` : '—';
});

const healthy = computed(() => health.value?.status === 'ok');

async function loadHealth(): Promise<void> {
  healthLoading.value = true;
  healthError.value = '';
  try {
    health.value = await api<HealthPayload>('/healthz', { auth: false });
  } catch (err) {
    health.value = null;
    healthError.value = errorMessage(err);
    toastError(err, '编码器自检失败');
  } finally {
    healthLoading.value = false;
  }
}

onMounted(loadHealth);
</script>

<template>
  <div class="max-w-[820px] space-y-5 sm:space-y-6">
    <!-- Account -->
    <AppCard padding="lg">
      <template #title>
        <h2 class="text-base font-semibold text-ink">账号</h2>
        <p class="mt-0.5 text-xs text-muted">当前登录的管理员与登录密码。</p>
      </template>

      <div class="mb-6 flex items-center gap-3 rounded-[var(--radius-control)] border border-line bg-sunken/60 px-4 py-3">
        <span
          class="grid size-9 shrink-0 place-items-center rounded-full bg-brand-50 text-sm font-semibold text-brand-700 dark:bg-brand-500/12 dark:text-brand-200"
        >
          {{ (auth.username || '?').slice(0, 1).toUpperCase() }}
        </span>
        <div class="min-w-0 flex-1">
          <p class="truncate text-sm font-medium text-ink">{{ auth.username || '未登录' }}</p>
          <p class="text-xs text-subtle">系统内唯一的登录账号</p>
        </div>
        <span
          class="inline-flex items-center gap-1.5 rounded-full bg-emerald-50 px-2.5 py-1 text-[11px] font-medium text-emerald-700 ring-1 ring-emerald-600/20 ring-inset dark:bg-emerald-500/10 dark:text-emerald-300 dark:ring-emerald-400/25"
        >
          <span class="size-1.5 rounded-full bg-current opacity-70" />
          已登录
        </span>
      </div>

      <form class="space-y-4" novalidate @submit.prevent="changePassword">
        <h3 class="flex items-center gap-2 text-sm font-medium text-ink">
          <KeyRound class="size-4 text-subtle" />
          修改密码
        </h3>

        <AppInput
          v-model="current"
          label="当前密码"
          type="password"
          autocomplete="current-password"
          :disabled="saving"
        />
        <div class="grid gap-4 sm:grid-cols-2">
          <AppInput
            v-model="next"
            label="新密码"
            type="password"
            help="至少 8 位"
            autocomplete="new-password"
            :disabled="saving"
          />
          <AppInput
            v-model="confirm"
            label="确认新密码"
            type="password"
            autocomplete="new-password"
            :disabled="saving"
          />
        </div>

        <p
          v-if="formError"
          role="alert"
          class="animate-fade-in rounded-[var(--radius-control)] border border-rose-200 bg-rose-50 px-3 py-2.5 text-xs text-rose-700 dark:border-rose-400/25 dark:bg-rose-500/10 dark:text-rose-300"
        >
          {{ formError }}
        </p>

        <div class="flex justify-end pt-1">
          <AppButton type="submit" :loading="saving">更新密码</AppButton>
        </div>
      </form>
    </AppCard>

    <!-- Appearance -->
    <AppCard padding="lg">
      <template #title>
        <h2 class="text-base font-semibold text-ink">外观</h2>
        <p class="mt-0.5 text-xs text-muted">选择后台的配色主题。</p>
      </template>

      <div class="grid grid-cols-1 gap-2 sm:grid-cols-3">
        <button
          v-for="option in THEMES"
          :key="option.value"
          type="button"
          class="focus-ring flex items-center gap-3 rounded-[var(--radius-control)] border px-4 py-3 text-left text-sm font-medium transition-colors"
          :class="
            themePreference === option.value
              ? 'border-brand-400 bg-brand-50 text-brand-700 dark:border-brand-400/50 dark:bg-brand-500/12 dark:text-brand-200'
              : 'border-line bg-raised text-muted hover:border-line-strong hover:text-ink'
          "
          :aria-pressed="themePreference === option.value"
          @click="chooseTheme(option.value)"
        >
          <component :is="option.icon" class="size-4 shrink-0" />
          <span class="flex-1">{{ option.label }}</span>
          <CheckCircle2 v-if="themePreference === option.value" class="size-4 shrink-0" />
        </button>
      </div>

      <p class="mt-3 text-xs text-subtle">
        「跟随系统」不保存偏好,会随操作系统的浅色 / 深色设置自动切换。
      </p>
    </AppCard>

    <!-- System / codec -->
    <AppCard padding="lg">
      <template #title>
        <h2 class="text-base font-semibold text-ink">系统与编码器</h2>
        <p class="mt-0.5 text-xs text-muted">
          服务端启动时自检数独编码表,这里显示自检结果。
        </p>
      </template>
      <template #actions>
        <AppButton variant="ghost" size="sm" :loading="healthLoading" @click="loadHealth">
          <RefreshCw v-if="!healthLoading" class="size-4" />
          重新检测
        </AppButton>
      </template>

      <div v-if="healthLoading" class="space-y-2">
        <div v-for="i in 4" :key="i" class="skeleton h-10 rounded-[var(--radius-control)]" />
      </div>

      <div
        v-else-if="healthError"
        class="flex flex-col items-center rounded-[var(--radius-control)] border border-rose-200 bg-rose-50 px-4 py-6 text-center dark:border-rose-400/25 dark:bg-rose-500/10"
      >
        <AlertTriangle class="size-6 text-rose-600 dark:text-rose-300" />
        <p class="mt-3 text-sm font-medium text-rose-800 dark:text-rose-200">编码器自检未通过</p>
        <p class="mt-1 max-w-md text-xs text-rose-700 text-balance dark:text-rose-300">
          {{ healthError }}
        </p>
        <p class="mt-2 max-w-md text-xs text-rose-600/80 dark:text-rose-300/70">
          浏览器与服务端的编码表可能不一致,请先暂停发布问卷。
        </p>
      </div>

      <div v-else-if="health" class="space-y-4">
        <div
          class="flex items-center gap-3 rounded-[var(--radius-control)] border px-4 py-3"
          :class="
            healthy
              ? 'border-emerald-200 bg-emerald-50 dark:border-emerald-400/25 dark:bg-emerald-500/10'
              : 'border-amber-200 bg-amber-50 dark:border-amber-400/25 dark:bg-amber-500/10'
          "
        >
          <CheckCircle2
            class="size-5 shrink-0"
            :class="healthy ? 'text-emerald-600 dark:text-emerald-300' : 'text-amber-600 dark:text-amber-300'"
          />
          <div>
            <p
              class="text-sm font-medium"
              :class="healthy ? 'text-emerald-800 dark:text-emerald-200' : 'text-amber-800 dark:text-amber-200'"
            >
              {{ healthy ? '编码器工作正常' : `服务状态:${health.status}` }}
            </p>
            <p
              class="text-xs"
              :class="healthy ? 'text-emerald-700/80 dark:text-emerald-300/80' : 'text-amber-700/80 dark:text-amber-300/80'"
            >
              网格枚举、位置组合与校验位图三者一致。
            </p>
          </div>
        </div>

        <dl class="divide-y divide-line overflow-hidden rounded-[var(--radius-control)] border border-line">
          <div class="flex items-center gap-4 px-4 py-2.5">
            <dt class="flex w-40 shrink-0 items-center gap-2 text-xs text-muted">
              <Database class="size-3.5 text-subtle" />
              编码方案
            </dt>
            <dd class="min-w-0 flex-1 truncate font-mono text-xs text-ink">{{ health.codec }}</dd>
          </div>
          <div class="flex items-center gap-4 px-4 py-2.5">
            <dt class="flex w-40 shrink-0 items-center gap-2 text-xs text-muted">
              <Server class="size-3.5 text-subtle" />
              布局
            </dt>
            <dd class="min-w-0 flex-1 truncate font-mono text-xs text-ink">{{ health.layout }}</dd>
          </div>
          <div class="flex items-center gap-4 px-4 py-2.5">
            <dt class="w-40 shrink-0 pl-[22px] text-xs text-muted">合法终盘数</dt>
            <dd class="min-w-0 flex-1 tabular-nums text-xs text-ink">
              {{ formatNumber(health.grids) }}
            </dd>
          </div>
          <div class="flex items-center gap-4 px-4 py-2.5">
            <dt class="w-40 shrink-0 pl-[22px] text-xs text-muted">位置组合数</dt>
            <dd class="min-w-0 flex-1 tabular-nums text-xs text-ink">
              {{ formatNumber(health.positionSets) }}
            </dd>
          </div>
          <div class="flex items-center gap-4 px-4 py-2.5">
            <dt class="w-40 shrink-0 pl-[22px] text-xs text-muted">终盘表 SHA-256</dt>
            <dd
              class="min-w-0 flex-1 truncate font-mono text-xs text-ink"
              :title="health.gridsSha256"
            >
              {{ shortDigest }}
            </dd>
          </div>
        </dl>
      </div>
    </AppCard>
  </div>
</template>
