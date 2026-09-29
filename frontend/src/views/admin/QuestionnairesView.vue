<script setup lang="ts">
import {
  AlertTriangle,
  BarChart3,
  CircleSlash,
  ClipboardList,
  Inbox,
  Link2,
  Pencil,
  Plus,
  RefreshCw,
  Search,
  Send,
  Trash2,
  X,
} from 'lucide-vue-next';
import { computed, onMounted, ref } from 'vue';
import { RouterLink } from 'vue-router';

import { api } from '@/api/client';
import type { QuestionnaireStatus, QuestionnaireSummary } from '@/api/types';
import AppButton from '@/components/ui/AppButton.vue';
import AppCard from '@/components/ui/AppCard.vue';
import AppConfirm from '@/components/ui/AppConfirm.vue';
import EmptyState from '@/components/ui/EmptyState.vue';
import StatusPill from '@/components/ui/StatusPill.vue';
import { formatDate, formatNumber, relativeTime, slugifyPreview } from '@/lib/format';
import { errorMessage, toast, toastError } from '@/lib/toast';

type StatusFilter = 'all' | QuestionnaireStatus;

const STATUS_FILTERS: { value: StatusFilter; label: string }[] = [
  { value: 'all', label: '全部' },
  { value: 'published', label: '已发布' },
  { value: 'draft', label: '草稿' },
  { value: 'closed', label: '已关闭' },
];

const items = ref<QuestionnaireSummary[]>([]);
const loading = ref(true);
const error = ref('');

const search = ref('');
const status = ref<StatusFilter>('all');

/** The row currently mid-mutation, so only its own controls go busy. */
const busyId = ref<number | null>(null);
const pendingDelete = ref<QuestionnaireSummary | null>(null);
const deleting = ref(false);

const filtered = computed(() => {
  const term = search.value.trim().toLowerCase();
  return items.value.filter((item) => {
    if (status.value !== 'all' && item.status !== status.value) return false;
    if (!term) return true;
    return (
      item.title.toLowerCase().includes(term) || item.slug.toLowerCase().includes(term)
    );
  });
});

const hasFilters = computed(() => Boolean(search.value.trim()) || status.value !== 'all');

async function load(): Promise<void> {
  loading.value = true;
  error.value = '';
  try {
    const payload = await api<{ items: QuestionnaireSummary[] }>('/api/admin/questionnaires');
    items.value = payload.items ?? [];
  } catch (err) {
    error.value = errorMessage(err);
    toastError(err, '无法加载问卷列表');
  } finally {
    loading.value = false;
  }
}

/** Every mutation ends the same way: reload the list, then say what happened. */
async function setStatus(item: QuestionnaireSummary, next: QuestionnaireStatus): Promise<void> {
  busyId.value = item.id;
  try {
    await api<{ ok: boolean; status: string }>(`/api/admin/questionnaires/${item.id}/status`, {
      method: 'POST',
      body: { status: next },
    });
    toast.success(next === 'published' ? '问卷已发布' : '问卷已关闭', item.title);
    await load();
  } catch (err) {
    toastError(err, '变更状态失败');
  } finally {
    busyId.value = null;
  }
}

async function confirmDelete(): Promise<void> {
  const item = pendingDelete.value;
  if (!item) return;
  deleting.value = true;
  try {
    await api<{ ok: boolean }>(`/api/admin/questionnaires/${item.id}`, { method: 'DELETE' });
    toast.success('问卷已删除', item.title);
    pendingDelete.value = null;
    await load();
  } catch (err) {
    toastError(err, '删除失败');
  } finally {
    deleting.value = false;
  }
}

async function copyLink(item: QuestionnaireSummary): Promise<void> {
  const url = slugifyPreview(item.slug);
  try {
    await navigator.clipboard.writeText(url);
    toast.success('填写链接已复制', url);
  } catch {
    // The clipboard API needs a secure context; on a plain-http host the
    // legacy path is the only way the admin can still copy the link.
    if (legacyCopy(url)) toast.success('填写链接已复制', url);
    else toastError(new Error('浏览器拒绝了剪贴板访问'), '复制失败');
  }
}

function legacyCopy(text: string): boolean {
  const el = document.createElement('textarea');
  el.value = text;
  el.setAttribute('readonly', '');
  el.style.position = 'fixed';
  el.style.opacity = '0';
  document.body.appendChild(el);
  el.select();
  let ok = false;
  try {
    ok = document.execCommand('copy');
  } catch {
    ok = false;
  }
  el.remove();
  return ok;
}

function clearFilters(): void {
  search.value = '';
  status.value = 'all';
}

onMounted(load);
</script>

<template>
  <div class="space-y-5">
    <!-- Toolbar -->
    <div class="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
      <div class="relative w-full lg:max-w-xs">
        <Search class="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-subtle" />
        <input
          v-model="search"
          type="search"
          placeholder="搜索标题或 slug…"
          aria-label="搜索问卷"
          class="focus-ring h-10 w-full rounded-[var(--radius-control)] border border-line bg-raised pl-9 pr-3 text-sm text-ink outline-none placeholder:text-subtle"
        />
      </div>

      <div class="flex flex-wrap items-center gap-2">
        <div class="flex items-center gap-0.5 rounded-[var(--radius-control)] border border-line bg-sunken p-0.5">
          <button
            v-for="option in STATUS_FILTERS"
            :key="option.value"
            type="button"
            class="rounded-lg px-3 py-1.5 text-[13px] font-medium transition-colors"
            :class="
              status === option.value
                ? 'bg-raised text-ink shadow-sm'
                : 'text-muted hover:text-ink'
            "
            :aria-pressed="status === option.value"
            @click="status = option.value"
          >
            {{ option.label }}
          </button>
        </div>

        <RouterLink
          to="/admin/questionnaires/new"
          class="inline-flex h-10 items-center gap-2 rounded-[var(--radius-control)] bg-brand-600 px-4 text-sm font-medium whitespace-nowrap text-white shadow-sm transition-[background-color,transform] duration-200 hover:bg-brand-500 active:scale-[0.985] dark:bg-brand-500 dark:hover:bg-brand-400"
        >
          <Plus class="size-4" />
          新建问卷
        </RouterLink>
      </div>
    </div>

    <!-- Failure -->
    <AppCard v-if="error && !loading" padding="lg">
      <div class="flex flex-col items-center py-6 text-center">
        <span class="grid size-12 place-items-center rounded-2xl bg-rose-50 text-rose-600 dark:bg-rose-500/12 dark:text-rose-300">
          <AlertTriangle class="size-6" />
        </span>
        <h2 class="mt-4 text-base font-semibold text-ink">问卷列表加载失败</h2>
        <p class="mt-1.5 max-w-sm text-sm text-muted text-balance">{{ error }}</p>
        <AppButton class="mt-5" variant="secondary" @click="load">
          <RefreshCw class="size-4" />
          重试
        </AppButton>
      </div>
    </AppCard>

    <template v-else>
      <!-- Loading -->
      <AppCard v-if="loading" padding="none">
        <div class="divide-y divide-line">
          <div v-for="i in 6" :key="i" class="flex items-center gap-4 px-4 py-4">
            <div class="skeleton h-4 w-1/3 rounded" />
            <div class="skeleton ml-auto h-4 w-20 rounded" />
            <div class="skeleton h-8 w-24 rounded-lg" />
          </div>
        </div>
      </AppCard>

      <!-- Nothing at all -->
      <AppCard v-else-if="!items.length" padding="none">
        <EmptyState
          :icon="ClipboardList"
          title="还没有问卷"
          description="创建第一份问卷,发布后即可生成填写链接与二维码。"
        >
          <RouterLink
            to="/admin/questionnaires/new"
            class="inline-flex h-10 items-center gap-2 rounded-[var(--radius-control)] bg-brand-600 px-4 text-sm font-medium text-white shadow-sm transition hover:bg-brand-500 dark:bg-brand-500 dark:hover:bg-brand-400"
          >
            <Plus class="size-4" />
            新建问卷
          </RouterLink>
        </EmptyState>
      </AppCard>

      <!-- Filtered to nothing -->
      <AppCard v-else-if="!filtered.length" padding="none">
        <EmptyState
          :icon="Search"
          title="没有匹配的问卷"
          description="换一个关键词或清除筛选条件再试一次。"
        >
          <AppButton variant="secondary" @click="clearFilters">
            <X class="size-4" />
            清除筛选
          </AppButton>
        </EmptyState>
      </AppCard>

      <template v-else>
        <!-- Desktop table -->
        <AppCard class="hidden lg:block" padding="none">
          <div class="overflow-x-auto">
            <table class="w-full min-w-[900px] text-sm">
              <thead>
                <tr class="border-b border-line text-left text-xs font-medium text-subtle">
                  <th scope="col" class="px-5 py-3 font-medium">标题</th>
                  <th scope="col" class="px-4 py-3 font-medium">状态</th>
                  <th scope="col" class="px-4 py-3 text-right font-medium">题目数</th>
                  <th scope="col" class="px-4 py-3 text-right font-medium">答卷数</th>
                  <th scope="col" class="px-4 py-3 font-medium">更新时间</th>
                  <th scope="col" class="px-5 py-3 text-right font-medium">操作</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-line">
                <tr
                  v-for="item in filtered"
                  :key="item.id"
                  class="transition-colors hover:bg-sunken/60"
                  :class="busyId === item.id ? 'opacity-60' : ''"
                >
                  <td class="max-w-[280px] px-5 py-3.5">
                    <RouterLink
                      :to="`/admin/questionnaires/${item.id}/edit`"
                      class="block truncate font-medium text-ink transition hover:text-brand-600 dark:hover:text-brand-300"
                    >
                      {{ item.title }}
                    </RouterLink>
                    <p class="truncate font-mono text-[11px] text-subtle">/q/{{ item.slug }}</p>
                  </td>
                  <td class="px-4 py-3.5">
                    <StatusPill :status="item.status" size="sm" />
                  </td>
                  <td class="px-4 py-3.5 text-right tabular-nums text-muted">
                    {{ formatNumber(item.questionCount) }}
                  </td>
                  <td class="px-4 py-3.5 text-right tabular-nums text-muted">
                    {{ formatNumber(item.responseCount) }}
                  </td>
                  <td class="px-4 py-3.5 whitespace-nowrap text-muted" :title="formatDate(item.updatedAt)">
                    {{ relativeTime(item.updatedAt) }}
                  </td>
                  <td class="px-5 py-3.5">
                    <div class="flex items-center justify-end gap-0.5">
                      <RouterLink
                        :to="`/admin/questionnaires/${item.id}/edit`"
                        class="icon-action"
                        title="编辑"
                        aria-label="编辑"
                      >
                        <Pencil class="size-4" />
                      </RouterLink>
                      <RouterLink
                        :to="`/admin/questionnaires/${item.id}/responses`"
                        class="icon-action"
                        title="答卷"
                        aria-label="答卷"
                      >
                        <Inbox class="size-4" />
                      </RouterLink>
                      <RouterLink
                        :to="`/admin/questionnaires/${item.id}/stats`"
                        class="icon-action"
                        title="统计"
                        aria-label="统计"
                      >
                        <BarChart3 class="size-4" />
                      </RouterLink>
                      <button
                        v-if="item.status === 'published'"
                        type="button"
                        class="icon-action"
                        title="关闭问卷"
                        aria-label="关闭问卷"
                        :disabled="busyId === item.id"
                        @click="setStatus(item, 'closed')"
                      >
                        <CircleSlash class="size-4" />
                      </button>
                      <button
                        v-else
                        type="button"
                        class="icon-action hover:text-emerald-600 dark:hover:text-emerald-400"
                        title="发布问卷"
                        aria-label="发布问卷"
                        :disabled="busyId === item.id"
                        @click="setStatus(item, 'published')"
                      >
                        <Send class="size-4" />
                      </button>
                      <button
                        type="button"
                        class="icon-action"
                        title="复制填写链接"
                        aria-label="复制填写链接"
                        @click="copyLink(item)"
                      >
                        <Link2 class="size-4" />
                      </button>
                      <button
                        type="button"
                        class="icon-action hover:text-rose-600 dark:hover:text-rose-400"
                        title="删除"
                        aria-label="删除"
                        @click="pendingDelete = item"
                      >
                        <Trash2 class="size-4" />
                      </button>
                    </div>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </AppCard>

        <!-- Mobile cards -->
        <div class="space-y-3 lg:hidden">
          <article
            v-for="item in filtered"
            :key="item.id"
            class="card p-4"
            :class="busyId === item.id ? 'opacity-60' : ''"
          >
            <div class="flex items-start justify-between gap-3">
              <div class="min-w-0">
                <RouterLink
                  :to="`/admin/questionnaires/${item.id}/edit`"
                  class="block truncate font-medium text-ink"
                >
                  {{ item.title }}
                </RouterLink>
                <p class="truncate font-mono text-[11px] text-subtle">/q/{{ item.slug }}</p>
              </div>
              <StatusPill :status="item.status" size="sm" />
            </div>

            <dl class="mt-3 flex items-center gap-4 text-xs text-muted">
              <div class="flex items-center gap-1">
                <dt>题目</dt>
                <dd class="tabular-nums font-medium text-ink">{{ item.questionCount }}</dd>
              </div>
              <div class="flex items-center gap-1">
                <dt>答卷</dt>
                <dd class="tabular-nums font-medium text-ink">{{ item.responseCount }}</dd>
              </div>
              <div class="ml-auto text-subtle">{{ relativeTime(item.updatedAt) }}</div>
            </dl>

            <div class="mt-3 flex flex-wrap gap-2 border-t border-line pt-3">
              <RouterLink
                :to="`/admin/questionnaires/${item.id}/edit`"
                class="mobile-action"
              >
                <Pencil class="size-3.5" />编辑
              </RouterLink>
              <RouterLink
                :to="`/admin/questionnaires/${item.id}/responses`"
                class="mobile-action"
              >
                <Inbox class="size-3.5" />答卷
              </RouterLink>
              <RouterLink
                :to="`/admin/questionnaires/${item.id}/stats`"
                class="mobile-action"
              >
                <BarChart3 class="size-3.5" />统计
              </RouterLink>
              <button
                v-if="item.status === 'published'"
                type="button"
                class="mobile-action"
                :disabled="busyId === item.id"
                @click="setStatus(item, 'closed')"
              >
                <CircleSlash class="size-3.5" />关闭
              </button>
              <button
                v-else
                type="button"
                class="mobile-action"
                :disabled="busyId === item.id"
                @click="setStatus(item, 'published')"
              >
                <Send class="size-3.5" />发布
              </button>
              <button type="button" class="mobile-action" @click="copyLink(item)">
                <Link2 class="size-3.5" />链接
              </button>
              <button
                type="button"
                class="mobile-action text-rose-600 dark:text-rose-400"
                @click="pendingDelete = item"
              >
                <Trash2 class="size-3.5" />删除
              </button>
            </div>
          </article>
        </div>

        <p v-if="hasFilters" class="px-1 text-xs text-subtle">
          共 {{ filtered.length }} / {{ items.length }} 份问卷
        </p>
      </template>
    </template>

    <AppConfirm
      :open="Boolean(pendingDelete)"
      title="删除问卷"
      :message="`确定要删除「${pendingDelete?.title ?? ''}」吗?该问卷的所有答卷与图片都会被一并删除,且无法恢复。`"
      confirm-text="删除"
      cancel-text="取消"
      danger
      :loading="deleting"
      @confirm="confirmDelete"
      @cancel="pendingDelete = null"
    />
  </div>
</template>

<style scoped>
/* Row actions stay near-invisible until the row is hovered, so the table reads
   as content rather than as a wall of buttons. */
.icon-action {
  display: grid;
  place-items: center;
  width: 2rem;
  height: 2rem;
  border-radius: 0.5rem;
  color: var(--text-subtle);
  transition:
    background-color 0.2s ease,
    color 0.2s ease;
}
.icon-action:hover {
  background-color: var(--surface-sunken);
  color: var(--text);
}
.icon-action:disabled {
  pointer-events: none;
  opacity: 0.4;
}

.mobile-action {
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
  border-radius: 0.5rem;
  border: 1px solid var(--border);
  padding: 0.35rem 0.6rem;
  font-size: 12px;
  font-weight: 500;
  color: var(--text-muted);
  transition:
    background-color 0.2s ease,
    border-color 0.2s ease,
    color 0.2s ease;
}
.mobile-action:hover {
  border-color: var(--border-strong);
  background-color: var(--surface-sunken);
  color: var(--text);
}
.mobile-action:disabled {
  pointer-events: none;
  opacity: 0.45;
}
</style>
