<script setup lang="ts">
/**
 * ResponsesView — browse, inspect, edit and delete the decoded answers.
 *
 * The table is the plaintext view of records the database only holds as
 * ciphertext: the list endpoint decodes on read. The one place ciphertext
 * surfaces is the 「查看原始密文」 action, which exists precisely so the admin
 * can see the difference for themselves.
 */
import {
  Binary,
  Check,
  ChevronLeft,
  ChevronRight,
  Copy,
  Download,
  Eye,
  Pencil,
  Trash2,
  TriangleAlert,
} from 'lucide-vue-next';
import { computed, onMounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';

import { api, downloadFile, getToken } from '@/api/client';
import type { ImageView, Question, QuestionnaireDetail, RawResponse, ResponseView } from '@/api/types';
import AppButton from '@/components/ui/AppButton.vue';
import AppCard from '@/components/ui/AppCard.vue';
import AppConfirm from '@/components/ui/AppConfirm.vue';
import AppInput from '@/components/ui/AppInput.vue';
import AppModal from '@/components/ui/AppModal.vue';
import AppSelect from '@/components/ui/AppSelect.vue';
import AppSkeleton from '@/components/ui/AppSkeleton.vue';
import EmptyState from '@/components/ui/EmptyState.vue';
import StatusPill from '@/components/ui/StatusPill.vue';
// The respondent-facing controls, reused so the edit form is the same UI the
// respondent filled in.
import AppCheckboxGroup from '@/components/ui/AppCheckboxGroup.vue';
import AppDateField from '@/components/ui/AppDateField.vue';
import AppRadioGroup from '@/components/ui/AppRadioGroup.vue';
import AppRating from '@/components/ui/AppRating.vue';
import AppTextarea from '@/components/ui/AppTextarea.vue';
import {
  decodeError,
  displayAnswer,
  formatDate,
  formatDuration,
  hasDecodeError,
  isAnswered,
  typeLabel,
} from '@/lib/format';
import { errorMessage, toast, toastError } from '@/lib/toast';

/** One server page of responses. */
interface ResponsePage {
  items: ResponseView[];
  total: number;
  page: number;
  size: number;
}

const route = useRoute();
const router = useRouter();

const id = computed(() => Number(typeof route.params.id === 'string' ? route.params.id : '0'));

const detail = ref<QuestionnaireDetail | null>(null);
const loading = ref(true);
const loadError = ref('');
const rows = ref<ResponseView[]>([]);
const total = ref(0);
const page = ref(1);
const size = ref(20);
const order = ref<'desc' | 'asc'>('desc');
const search = ref('');

const detailModalOpen = ref(false);
const current = ref<ResponseView | null>(null);

const editModalOpen = ref(false);
const editAnswers = ref<Record<string, unknown>>({});
const savingEdit = ref(false);

const rawModalOpen = ref(false);
const raw = ref<RawResponse | null>(null);
const rawLoading = ref(false);
const rawError = ref('');

const deleteOpen = ref(false);
const deleting = ref(false);
const copiedRaw = ref(false);

/** A search widens the scan window: paging 20 at a time would hide matches. */
const SEARCH_SCAN = 200;
const scanned = ref(0);

const questions = computed<Question[]>(
  () => detail.value?.definition?.steps.flatMap((s) => s.questions) ?? [],
);
const pageCount = computed(() => Math.max(1, Math.ceil(total.value / size.value)));

/** Answers of the on-screen page, narrowed by the search box. */
const visibleRows = computed(() => {
  const term = search.value.trim().toLowerCase();
  if (!term) return rows.value;
  return rows.value.filter((row) => {
    if (String(row.index).includes(term)) return true;
    if (formatDate(row.createdAt).toLowerCase().includes(term)) return true;
    return questions.value.some((q) =>
      displayAnswer(q, row.answers[q.key]).toLowerCase().includes(term),
    );
  });
});

/**
 * The respondent-facing controls, keyed by type. Each declares its own prop
 * shape, so `editAttrs` supplies exactly what the type in question takes.
 */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const CONTROLS: Record<string, any> = {
  single: AppRadioGroup,
  multi: AppCheckboxGroup,
  select: AppSelect,
  text: AppInput,
  textarea: AppTextarea,
  rating: AppRating,
  date: AppDateField,
};

/** These controls draw their own label; the rest are labelled by the parent. */
const OWNS_LABEL = ['select', 'text', 'textarea', 'date'];

const SIZE_OPTIONS = [
  { value: '10', label: '每页 10 条' },
  { value: '20', label: '每页 20 条' },
  { value: '50', label: '每页 50 条' },
  { value: '100', label: '每页 100 条' },
];

const EXPORT_FORMATS = [
  { format: 'csv', label: 'CSV', ext: 'csv' },
  { format: 'xlsx', label: 'Excel', ext: 'xlsx' },
  { format: 'json', label: 'JSON', ext: 'json' },
];

async function loadDetail() {
  try {
    detail.value = await api<QuestionnaireDetail>(`/api/admin/questionnaires/${id.value}`);
  } catch (err) {
    loadError.value = errorMessage(err);
  }
}

async function loadRows() {
  loading.value = true;
  loadError.value = '';
  try {
    // A search has no server-side counterpart, so it scans a wider window.
    const effectiveSize = search.value.trim() ? SEARCH_SCAN : size.value;
    const effectivePage = search.value.trim() ? 1 : page.value;
    const query = new URLSearchParams({
      page: String(effectivePage),
      size: String(effectiveSize),
      order: order.value,
    });
    const data = await api<ResponsePage>(`/api/admin/questionnaires/${id.value}/responses?${query}`);
    rows.value = data.items;
    total.value = data.total;
    scanned.value = data.items.length;
  } catch (err) {
    loadError.value = errorMessage(err);
    rows.value = [];
  } finally {
    loading.value = false;
  }
}

onMounted(async () => {
  await Promise.all([loadDetail(), loadRows()]);
});

watch([page, size, order], () => {
  void loadRows();
});

let searchTimer = 0;
watch(search, () => {
  window.clearTimeout(searchTimer);
  searchTimer = window.setTimeout(() => {
    page.value = 1;
    void loadRows();
  }, 300);
});

function refresh() {
  void loadRows();
}

// ------------------------------------------------------------------ images ---

/**
 * The image endpoint is authenticated. A plain <img> cannot send a header, so
 * the token rides in the query string — the backend accepts `?token=` for
 * exactly this case.
 */
function imageUrl(image: ImageView | number): string {
  const imageId = typeof image === 'number' ? image : image.id;
  const token = getToken();
  return `/api/admin/images/${imageId}${token ? `?token=${encodeURIComponent(token)}` : ''}`;
}

function imagesFor(row: ResponseView, key: string): ImageView[] {
  return row.images.filter((i) => i.fieldKey === key);
}

// ------------------------------------------------------------- detail/edit ---

function openDetail(row: ResponseView) {
  current.value = row;
  detailModalOpen.value = true;
}

function openEdit(row: ResponseView) {
  if (hasDecodeError(row.answers)) {
    toast.error('无法编辑', '这份答卷的密文未被解码,请先排查密钥或数据损坏');
    return;
  }
  current.value = row;
  editAnswers.value = { ...row.answers };
  editModalOpen.value = true;
}

/** Hands the open detail modal over to the edit modal. */
function editFromDetail() {
  const row = current.value;
  if (!row) return;
  detailModalOpen.value = false;
  openEdit(row);
}

/** The props a control needs for this question, bound to the draft answers. */
function editAttrs(q: Question): Record<string, unknown> {
  const value = editAnswers.value[q.key];
  const label = q.label;
  const help = q.help?.trim() || undefined;
  const binding = {
    'onUpdate:modelValue': (next: unknown) => {
      editAnswers.value[q.key] = next;
    },
  };
  const text = typeof value === 'string' ? value : '';
  switch (q.type) {
    case 'single':
      return { ...binding, modelValue: text, options: q.options ?? [] };
    case 'multi':
      return { ...binding, modelValue: Array.isArray(value) ? value : [], options: q.options ?? [] };
    case 'select':
      return { ...binding, modelValue: text, label, help, options: q.options ?? [] };
    case 'text':
      return { ...binding, modelValue: text, label, help, maxlength: q.maxLength };
    case 'textarea':
      return { ...binding, modelValue: text, label, help, maxlength: q.maxLength, rows: 3 };
    case 'rating':
      return {
        ...binding,
        modelValue: typeof value === 'number' ? value : null,
        min: q.min ?? 1,
        max: q.max ?? 5,
      };
    case 'date':
      return { ...binding, modelValue: text, label, help };
    default:
      return binding;
  }
}

/** Empty answers are sent as null so the server sees "unanswered". */
function collectAnswers(): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  for (const q of questions.value) {
    const value = editAnswers.value[q.key];
    out[q.key] = isAnswered(value) ? value : null;
  }
  return out;
}

async function saveEdit() {
  if (!current.value) return;
  savingEdit.value = true;
  try {
    await api(`/api/admin/questionnaires/${id.value}/responses/${current.value.id}`, {
      method: 'PUT',
      body: { answers: collectAnswers() },
    });
    toast.success('已保存', '修改后的答案重新编码后写回');
    editModalOpen.value = false;
    await loadRows();
  } catch (err) {
    toastError(err, '保存失败');
  } finally {
    savingEdit.value = false;
  }
}

// ------------------------------------------------------------------- raw ---

async function openRaw(row: ResponseView) {
  current.value = row;
  rawModalOpen.value = true;
  raw.value = null;
  rawError.value = '';
  copiedRaw.value = false;
  await loadRaw();
}

async function loadRaw() {
  if (!current.value) return;
  rawLoading.value = true;
  rawError.value = '';
  try {
    raw.value = await api<RawResponse>(
      `/api/admin/questionnaires/${id.value}/responses/${current.value.id}/raw`,
    );
  } catch (err) {
    rawError.value = errorMessage(err);
  } finally {
    rawLoading.value = false;
  }
}

async function copyCipher() {
  if (!raw.value) return;
  try {
    await navigator.clipboard.writeText(raw.value.cipher);
    copiedRaw.value = true;
    toast.success('密文已复制');
    window.setTimeout(() => (copiedRaw.value = false), 2000);
  } catch {
    toast.error('复制失败', '请手动选中密文复制');
  }
}

// ----------------------------------------------------------------- delete ---

function askDelete(row: ResponseView) {
  current.value = row;
  deleteOpen.value = true;
}

async function confirmDelete() {
  if (!current.value) return;
  deleting.value = true;
  try {
    await api(`/api/admin/questionnaires/${id.value}/responses/${current.value.id}`, {
      method: 'DELETE',
    });
    toast.success('已删除');
    deleteOpen.value = false;
    // Stepping back a page keeps the admin from landing on an empty list.
    if (rows.value.length === 1 && page.value > 1) page.value -= 1;
    else await loadRows();
  } catch (err) {
    toastError(err, '删除失败');
  } finally {
    deleting.value = false;
  }
}

// ----------------------------------------------------------------- export ---

async function exportAs(format: string, ext: string) {
  try {
    await downloadFile(
      `/api/admin/questionnaires/${id.value}/export?format=${format}`,
      `${detail.value?.slug ?? 'export'}.${ext}`,
    );
  } catch (err) {
    toastError(err, '导出失败');
  }
}

const shareUrl = computed(() =>
  detail.value ? `${window.location.origin}/q/${detail.value.slug}` : '',
);

async function copyShare() {
  if (!shareUrl.value) return;
  try {
    await navigator.clipboard.writeText(shareUrl.value);
    toast.success('链接已复制');
  } catch {
    toast.error('复制失败', '请手动选中链接复制');
  }
}
</script>

<template>
  <div class="mx-auto w-full max-w-7xl">
    <header class="mb-5 flex flex-wrap items-start justify-between gap-3">
      <div class="min-w-0">
        <div class="flex flex-wrap items-center gap-2">
          <h1 class="text-xl font-semibold text-ink">
            {{ detail?.definition?.title || '答卷数据' }}
          </h1>
          <StatusPill v-if="detail" :status="detail.status" size="sm" />
        </div>
        <p class="mt-1 text-sm text-muted">
          共 <span class="font-semibold tabular-nums text-ink">{{ total }}</span> 份答卷 ·
          数据库中是数独密文,此处显示解码后的明文
        </p>
      </div>
      <div class="flex flex-wrap items-center gap-2">
        <AppButton
          v-if="detail"
          variant="secondary"
          @click="router.push({ name: 'admin-stats', params: { id } })"
        >
          统计分析
        </AppButton>
        <AppButton
          v-if="detail"
          variant="secondary"
          @click="router.push({ name: 'admin-questionnaire-edit', params: { id } })"
        >
          编辑问卷
        </AppButton>
      </div>
    </header>

    <!-- ------------------------------------------------------ toolbar --- -->
    <AppCard padding="sm" class="mb-4">
      <div class="flex flex-wrap items-center gap-3">
        <div class="min-w-[200px] flex-1">
          <AppInput v-model="search" placeholder="搜索答案内容…" />
        </div>
        <AppButton
          variant="secondary"
          size="sm"
          :title="order === 'desc' ? '当前:最新优先' : '当前:最早优先'"
          @click="order = order === 'desc' ? 'asc' : 'desc'"
        >
          {{ order === 'desc' ? '最新优先' : '最早优先' }}
        </AppButton>
        <div class="w-32">
          <AppSelect
            :model-value="String(size)"
            :options="SIZE_OPTIONS"
            @update:model-value="(v: string) => (size = Number(v))"
          />
        </div>
        <div class="flex items-center gap-1">
          <AppButton
            v-for="f in EXPORT_FORMATS"
            :key="f.format"
            variant="ghost"
            size="sm"
            @click="exportAs(f.format, f.ext)"
          >
            <Download class="size-3.5" />
            {{ f.label }}
          </AppButton>
        </div>
      </div>
      <p v-if="search.trim()" class="mt-2 text-xs text-subtle">
        搜索在扫描到的 {{ scanned }} 条内进行,匹配
        <span class="font-semibold tabular-nums text-muted">{{ visibleRows.length }}</span> 条。
      </p>
    </AppCard>

    <!-- ------------------------------------------------- load states --- -->
    <div v-if="loading" class="space-y-3">
      <AppSkeleton variant="card" :lines="4" />
      <AppSkeleton variant="card" :lines="4" />
    </div>

    <AppCard v-else-if="loadError" padding="lg">
      <EmptyState :icon="TriangleAlert" title="无法加载答卷" :description="loadError">
        <AppButton variant="secondary" @click="refresh">重试</AppButton>
      </EmptyState>
    </AppCard>

    <AppCard v-else-if="detail?.undecodable" padding="lg">
      <EmptyState
        :icon="TriangleAlert"
        title="问卷定义无法解码"
        :description="detail.error || '无法渲染列头,请先修复问卷定义。'"
      />
    </AppCard>

    <AppCard v-else-if="total === 0" padding="none">
      <EmptyState
        title="还没有答卷"
        description="把填写链接发给受访者,提交后会自动出现在这里。若问卷仍是草稿状态,受访者无法填写,请先发布。"
      >
        <div class="flex flex-wrap items-center justify-center gap-2">
          <AppButton variant="secondary" @click="copyShare">
            <Copy class="size-4" />
            复制链接
          </AppButton>
          <AppButton
            variant="ghost"
            @click="router.push({ name: 'admin-questionnaire-edit', params: { id } })"
          >
            去发布
          </AppButton>
        </div>
      </EmptyState>
    </AppCard>

    <template v-else>
      <!-- ------------------------------------------------------ table --- -->
      <AppCard padding="none" class="overflow-hidden">
        <div class="overflow-x-auto">
          <table class="w-full min-w-[720px] border-collapse text-sm">
            <thead>
              <tr class="border-b border-line bg-sunken/60 text-left">
                <th class="px-3 py-2.5 font-medium text-muted whitespace-nowrap">序号</th>
                <th class="px-3 py-2.5 font-medium text-muted whitespace-nowrap">提交时间</th>
                <th class="px-3 py-2.5 font-medium text-muted whitespace-nowrap">用时</th>
                <th
                  v-for="q in questions"
                  :key="q.key"
                  class="px-3 py-2.5 font-medium text-muted"
                  :title="`${q.key} · ${typeLabel(q.type)}`"
                >
                  <span class="block max-w-[220px] truncate">{{ q.label }}</span>
                </th>
                <th class="px-3 py-2.5 text-right font-medium text-muted whitespace-nowrap">操作</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="row in visibleRows"
                :key="row.id"
                class="cursor-pointer border-b border-line/70 transition-colors last:border-0 hover:bg-sunken/60"
                :class="hasDecodeError(row.answers) ? 'bg-rose-50/60 dark:bg-rose-500/5' : ''"
                @click="hasDecodeError(row.answers) ? openRaw(row) : openDetail(row)"
              >
                <td class="px-3 py-2.5 font-mono text-xs text-subtle tabular-nums">
                  {{ row.index }}
                </td>
                <td class="px-3 py-2.5 whitespace-nowrap text-muted">
                  {{ formatDate(row.createdAt) }}
                </td>
                <td class="px-3 py-2.5 whitespace-nowrap text-muted tabular-nums">
                  {{ formatDuration(row.durationMs) }}
                </td>

                <!-- an undecodable row carries one message, not a set of answers -->
                <td v-if="hasDecodeError(row.answers)" :colspan="questions.length" class="px-3 py-2.5">
                  <div class="flex items-start gap-2 text-rose-700 dark:text-rose-300">
                    <TriangleAlert class="mt-0.5 size-4 shrink-0" />
                    <span class="text-xs">
                      <span class="font-medium">无法解码:</span>{{ decodeError(row.answers) }}
                    </span>
                  </div>
                </td>

                <template v-else>
                  <td v-for="q in questions" :key="q.key" class="px-3 py-2.5 align-top">
                    <!-- images get thumbnails; the token rides in the query -->
                    <div v-if="imagesFor(row, q.key).length" class="flex flex-wrap gap-1">
                      <img
                        v-for="img in imagesFor(row, q.key)"
                        :key="img.id"
                        :src="imageUrl(img)"
                        :alt="q.label"
                        loading="lazy"
                        decoding="async"
                        class="size-10 rounded-md border border-line object-cover"
                      />
                    </div>
                    <span
                      v-else
                      class="block max-w-[220px] truncate text-ink"
                      :title="displayAnswer(q, row.answers[q.key])"
                    >
                      {{ displayAnswer(q, row.answers[q.key]) || '—' }}
                    </span>
                  </td>
                </template>

                <td class="px-3 py-2.5 text-right whitespace-nowrap">
                  <div class="flex items-center justify-end gap-1" @click.stop>
                    <AppButton
                      variant="ghost"
                      size="sm"
                      title="查看详情"
                      :disabled="hasDecodeError(row.answers)"
                      @click="openDetail(row)"
                    >
                      <Eye class="size-3.5" />
                    </AppButton>
                    <AppButton
                      variant="ghost"
                      size="sm"
                      title="编辑答案"
                      :disabled="hasDecodeError(row.answers)"
                      @click="openEdit(row)"
                    >
                      <Pencil class="size-3.5" />
                    </AppButton>
                    <AppButton variant="ghost" size="sm" title="查看原始密文" @click="openRaw(row)">
                      <Binary class="size-3.5" />
                    </AppButton>
                    <AppButton variant="ghost" size="sm" title="删除" @click="askDelete(row)">
                      <Trash2 class="size-3.5" />
                    </AppButton>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <div v-if="visibleRows.length === 0" class="px-4 py-10">
          <EmptyState title="没有匹配的答卷" description="换个关键词,或清空搜索框。" />
        </div>
      </AppCard>

      <!-- ------------------------------------------------- pagination --- -->
      <div class="mt-4 flex flex-wrap items-center justify-between gap-3">
        <p class="text-sm text-muted tabular-nums">
          共 {{ total }} 条 · 第 {{ page }} / {{ pageCount }} 页
        </p>
        <div class="flex items-center gap-2">
          <AppButton variant="secondary" size="sm" :disabled="page <= 1" @click="page -= 1">
            <ChevronLeft class="size-4" />
            上一页
          </AppButton>
          <AppButton variant="secondary" size="sm" :disabled="page >= pageCount" @click="page += 1">
            下一页
            <ChevronRight class="size-4" />
          </AppButton>
        </div>
      </div>
    </template>

    <!-- ------------------------------------------------------ detail --- -->
    <AppModal
      :open="detailModalOpen"
      size="lg"
      title="答卷详情"
      :description="current ? `第 ${current.index} 号 · ${formatDate(current.createdAt)}` : ''"
      @close="detailModalOpen = false"
    >
      <div v-if="current" class="space-y-5">
        <dl class="grid grid-cols-2 gap-3 text-sm sm:grid-cols-4">
          <div>
            <dt class="text-xs text-subtle">提交时间</dt>
            <dd class="mt-0.5 text-ink">{{ formatDate(current.createdAt) }}</dd>
          </div>
          <div>
            <dt class="text-xs text-subtle">最后修改</dt>
            <dd class="mt-0.5 text-ink">{{ formatDate(current.updatedAt) }}</dd>
          </div>
          <div>
            <dt class="text-xs text-subtle">用时</dt>
            <dd class="mt-0.5 text-ink tabular-nums">{{ formatDuration(current.durationMs) }}</dd>
          </div>
          <div>
            <dt class="text-xs text-subtle">图片</dt>
            <dd class="mt-0.5 text-ink tabular-nums">{{ current.images.length }} 张</dd>
          </div>
        </dl>

        <div class="space-y-4 border-t border-line pt-4">
          <div v-for="q in questions" :key="q.key">
            <p class="text-sm font-medium text-ink">
              {{ q.label }}
              <span class="ml-1 font-mono text-xs font-normal text-subtle">{{ q.key }}</span>
            </p>
            <p v-if="q.help" class="mt-0.5 text-xs text-subtle">{{ q.help }}</p>

            <div v-if="imagesFor(current, q.key).length" class="mt-2 flex flex-wrap gap-2">
              <img
                v-for="img in imagesFor(current, q.key)"
                :key="img.id"
                :src="imageUrl(img)"
                :alt="q.label"
                loading="lazy"
                class="max-h-72 rounded-lg border border-line object-contain"
              />
            </div>
            <p v-else class="mt-1 text-sm whitespace-pre-wrap text-muted">
              {{ displayAnswer(q, current.answers[q.key]) || '未作答' }}
            </p>
          </div>
        </div>
      </div>

      <template #footer>
        <div class="flex justify-end gap-2">
          <AppButton variant="ghost" @click="detailModalOpen = false">关闭</AppButton>
          <AppButton
            v-if="current && !hasDecodeError(current.answers)"
            variant="secondary"
            @click="editFromDetail"
          >
            <Pencil class="size-4" />
            编辑
          </AppButton>
        </div>
      </template>
    </AppModal>

    <!-- -------------------------------------------------------- edit --- -->
    <AppModal
      :open="editModalOpen"
      size="lg"
      title="编辑答案"
      description="保存后重新编码写回,数据库里仍然只有密文"
      @close="editModalOpen = false"
    >
      <div v-if="current" class="space-y-5">
        <div v-for="q in questions" :key="q.key">
          <div
            v-if="imagesFor(current, q.key).length"
            class="rounded-[var(--radius-control)] border border-line bg-sunken/60 p-3"
          >
            <p class="text-sm font-medium text-ink">{{ q.label }}</p>
            <div class="mt-2 flex flex-wrap gap-2">
              <img
                v-for="img in imagesFor(current, q.key)"
                :key="img.id"
                :src="imageUrl(img)"
                :alt="q.label"
                loading="lazy"
                class="max-h-40 rounded-lg border border-line object-contain"
              />
            </div>
            <p class="mt-2 text-xs text-subtle">图片不可在此修改,如需更换请让受访者重新上传。</p>
          </div>
          <div v-else>
            <!-- radio / checkbox / rating draw no label of their own -->
            <div v-if="!OWNS_LABEL.includes(q.type)" class="mb-1.5">
              <p class="text-sm font-medium text-ink">
                {{ q.label }}
                <span v-if="q.required" class="ml-0.5 text-rose-500" aria-hidden="true">*</span>
              </p>
              <p v-if="q.help" class="mt-0.5 text-xs text-subtle">{{ q.help }}</p>
            </div>
            <component :is="CONTROLS[q.type]" v-bind="editAttrs(q)" />
          </div>
        </div>
      </div>

      <template #footer>
        <div class="flex justify-end gap-2">
          <AppButton variant="ghost" @click="editModalOpen = false">取消</AppButton>
          <AppButton :loading="savingEdit" @click="saveEdit">保存</AppButton>
        </div>
      </template>
    </AppModal>

    <!-- --------------------------------------------------------- raw --- -->
    <AppModal
      :open="rawModalOpen"
      size="lg"
      title="原始密文"
      :description="current ? `第 ${current.index} 号答卷在数据库中的样子` : ''"
      @close="rawModalOpen = false"
    >
      <div v-if="rawLoading" class="space-y-3">
        <AppSkeleton variant="text" :lines="4" />
        <AppSkeleton variant="text" :lines="2" />
      </div>

      <div v-else-if="rawError" class="py-4">
        <EmptyState :icon="TriangleAlert" title="读取失败" :description="rawError">
          <AppButton variant="secondary" @click="loadRaw">重试</AppButton>
        </EmptyState>
      </div>

      <div v-else-if="raw" class="space-y-4">
        <div class="flex flex-wrap items-center gap-2 text-xs">
          <span class="rounded-full bg-sunken px-2 py-1 font-mono text-muted">
            layout: {{ raw.layout }}
          </span>
          <span class="rounded-full bg-sunken px-2 py-1 font-mono text-muted">
            {{ raw.length }} 字节
          </span>
          <span
            class="rounded-full px-2 py-1 font-mono"
            :class="
              raw.inspection.checksumOk
                ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-300'
                : 'bg-rose-50 text-rose-700 dark:bg-rose-500/10 dark:text-rose-300'
            "
          >
            checksum {{ raw.inspection.checksumOk ? 'OK' : 'FAIL' }}
          </span>
          <AppButton variant="ghost" size="sm" class="ml-auto" @click="copyCipher">
            <component :is="copiedRaw ? Check : Copy" class="size-3.5" />
            {{ copiedRaw ? '已复制' : '复制密文' }}
          </AppButton>
        </div>

        <div
          data-cipher
          class="cipher max-h-72 overflow-y-auto rounded-[var(--radius-control)] border border-line bg-sunken p-3 text-muted"
        >
          {{ raw.cipher }}
        </div>

        <dl class="grid grid-cols-2 gap-3 text-sm sm:grid-cols-3">
          <div class="rounded-[var(--radius-control)] border border-line p-3">
            <dt class="text-xs text-subtle">线索符号数</dt>
            <dd class="mt-1 font-mono text-ink tabular-nums">{{ raw.inspection.clueSymbols }}</dd>
          </div>
          <div class="rounded-[var(--radius-control)] border border-line p-3">
            <dt class="text-xs text-subtle">填充字节</dt>
            <dd class="mt-1 font-mono text-ink tabular-nums">{{ raw.inspection.paddingBytes }}</dd>
          </div>
          <div class="rounded-[var(--radius-control)] border border-line p-3">
            <dt class="text-xs text-subtle">明文字节</dt>
            <dd class="mt-1 font-mono text-ink tabular-nums">{{ raw.inspection.plaintextLen }}</dd>
          </div>
          <div class="rounded-[var(--radius-control)] border border-line p-3">
            <dt class="text-xs text-subtle">校验和</dt>
            <dd class="mt-1 font-mono text-ink">
              {{ raw.inspection.checksumOk ? '通过' : '不匹配' }}
            </dd>
          </div>
          <div class="rounded-[var(--radius-control)] border border-line p-3">
            <dt class="text-xs text-subtle">尾帧</dt>
            <dd class="mt-1 font-mono text-xs break-all text-ink">
              {{ raw.inspection.trailerHex || '—' }}
            </dd>
          </div>
          <div class="rounded-[var(--radius-control)] border border-line p-3">
            <dt class="text-xs text-subtle">非法字节</dt>
            <dd class="mt-1 font-mono text-ink tabular-nums">
              {{ raw.inspection.rejectedByte >= 0 ? raw.inspection.rejectedByte : '无' }}
            </dd>
          </div>
        </dl>

        <p class="text-xs text-subtle">
          线索符号数 = 明文字节数 × 4 + 4(尾帧)。这就是数据库里真实存储的内容。
        </p>
      </div>

      <template #footer>
        <div class="flex justify-end">
          <AppButton variant="ghost" @click="rawModalOpen = false">关闭</AppButton>
        </div>
      </template>
    </AppModal>

    <AppConfirm
      :open="deleteOpen"
      title="删除这份答卷?"
      message="删除后无法恢复。密文与关联图片都会被移除。"
      confirm-text="删除"
      cancel-text="取消"
      danger
      :loading="deleting"
      @confirm="confirmDelete"
      @cancel="deleteOpen = false"
    />
  </div>
</template>
