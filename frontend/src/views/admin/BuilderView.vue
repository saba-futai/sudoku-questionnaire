<script setup lang="ts">
/**
 * BuilderView — create or edit a questionnaire definition.
 *
 * Everything on this screen is plaintext the administrator owns: the API hands
 * back a DECODED definition, and the payload we send is encoded server-side
 * before it is stored. The editor therefore never needs to know the codec
 * exists — except in one place, the "definition is stored as ciphertext" note,
 * so the admin understands why an undecodable row is even possible.
 */
import {
  AlertTriangle,
  ArrowDown,
  ArrowUp,
  Check,
  Copy,
  Eye,
  EyeOff,
  Image as ImageIcon,
  ListChecks,
  Plus,
  Save,
  Trash2,
} from 'lucide-vue-next';
import { computed, nextTick, onMounted, reactive, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';

import { api } from '@/api/client';
import type {
  Definition,
  Option,
  Question,
  QuestionnaireDetail,
  QuestionnaireStatus,
  QuestionnaireSummary,
  QuestionType,
} from '@/api/types';
import AppButton from '@/components/ui/AppButton.vue';
import AppCard from '@/components/ui/AppCard.vue';
import AppConfirm from '@/components/ui/AppConfirm.vue';
import AppInput from '@/components/ui/AppInput.vue';
import AppSelect from '@/components/ui/AppSelect.vue';
import AppSkeleton from '@/components/ui/AppSkeleton.vue';
import AppTextarea from '@/components/ui/AppTextarea.vue';
import EmptyState from '@/components/ui/EmptyState.vue';
import StatusPill from '@/components/ui/StatusPill.vue';
// The respondent-facing controls, reused verbatim in the preview: an admin
// should see the exact widget a respondent will get, not an approximation.
import AppCheckboxGroup from '@/components/ui/AppCheckboxGroup.vue';
import AppDateField from '@/components/ui/AppDateField.vue';
import AppRadioGroup from '@/components/ui/AppRadioGroup.vue';
import AppRating from '@/components/ui/AppRating.vue';
import QrCode from '@/components/ui/QrCode.vue';
import { QUESTION_TYPE_LABELS, typeLabel } from '@/lib/format';
import { errorMessage, toast, toastError } from '@/lib/toast';

/** A step while it is being edited: `description` is always a string here. */
interface DraftStep {
  id: string;
  title: string;
  description: string;
  questions: Question[];
}

/** Local shape of a definition under construction. */
interface Draft {
  title: string;
  description: string;
  status: QuestionnaireStatus;
  slug: string;
  steps: DraftStep[];
}

interface ValidationIssue {
  message: string;
  /** -1 when the problem is the questionnaire itself (e.g. a missing title). */
  stepIndex: number;
  questionIndex?: number;
}

const route = useRoute();
const router = useRouter();

const idParam = computed(() => (typeof route.params.id === 'string' ? route.params.id : ''));
const isEdit = computed(() => idParam.value !== '');
const numericId = computed(() => Number(idParam.value));

const loading = ref(false);
const loadError = ref('');
const saving = ref(false);
const savedAt = ref('');
/** Set after a successful create so the share panel can take over the page. */
const created = ref<{ id: number; slug: string } | null>(null);
const confirmOpen = ref(false);
const issue = ref<ValidationIssue | null>(null);
const issueCount = ref(0);
/** Below lg the two columns become tabs; this is which one is showing. */
const mobileTab = ref<'editor' | 'preview'>('editor');
const previewFilled = ref(false);
const copied = ref(false);

/** How many answers already exist — editing a live questionnaire can orphan them. */
const responseCount = ref(0);

const TYPE_OPTIONS = (Object.keys(QUESTION_TYPE_LABELS) as QuestionType[]).map((t) => ({
  value: t,
  label: typeLabel(t),
}));

const STATUS_OPTIONS: { value: QuestionnaireStatus; label: string }[] = [
  { value: 'draft', label: '草稿' },
  { value: 'published', label: '已发布' },
  { value: 'closed', label: '已关闭' },
];

const form = reactive<Draft>({
  title: '',
  description: '',
  status: 'draft',
  slug: '',
  steps: [],
});

/**
 * The controls a respondent sees, keyed by type. Held loosely on purpose: each
 * control declares its own prop shape, and the preview hands each one exactly
 * the attributes its own type takes (see `previewAttrs`).
 */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const CONTROLS: Record<QuestionType, any> = {
  single: AppRadioGroup,
  multi: AppCheckboxGroup,
  select: AppSelect,
  text: AppInput,
  textarea: AppTextarea,
  rating: AppRating,
  date: AppDateField,
  image: null,
};

/** These controls draw their own label; the rest are labelled by the parent. */
const OWNS_LABEL: QuestionType[] = ['select', 'text', 'textarea', 'date'];

const CHOICE: QuestionType[] = ['single', 'multi', 'select'];
const isChoice = (t: QuestionType) => CHOICE.includes(t);

const totalQuestions = computed(() => form.steps.reduce((n, s) => n + s.questions.length, 0));
const allKeys = computed(() => form.steps.flatMap((s) => s.questions.map((q) => q.key.trim())));

function defaultOptions(): Option[] {
  return [
    { value: '1', label: '选项 1' },
    { value: '2', label: '选项 2' },
  ];
}

function blankQuestion(key: string): Question {
  return {
    key,
    type: 'single',
    label: '',
    help: '',
    required: false,
    options: defaultOptions(),
  };
}

function nextKey(): string {
  const used = new Set(allKeys.value);
  let n = 1;
  while (used.has(`q${n}`)) n += 1;
  return `q${n}`;
}

function nextStepId(): string {
  const used = new Set(form.steps.map((s) => s.id));
  let n = 1;
  while (used.has(`step-${n}`)) n += 1;
  return `step-${n}`;
}

/** Brings a server definition into the editor's always-defined shape. */
function normalizeQuestion(q: Question): Question {
  const out: Question = {
    key: q.key,
    type: q.type,
    label: q.label,
    help: q.help ?? '',
    required: !!q.required,
  };
  if (isChoice(q.type)) {
    out.options = (q.options ?? []).map((o) => ({ value: o.value, label: o.label }));
  }
  if (q.type === 'text' || q.type === 'textarea') {
    if (q.minLength) out.minLength = q.minLength;
    if (q.maxLength) out.maxLength = q.maxLength;
  }
  if (q.type === 'rating') {
    out.min = q.min ?? 1;
    out.max = q.max ?? 5;
  }
  if (q.type === 'image') out.maxImages = q.maxImages ?? 1;
  return out;
}

function blankForm(): Draft {
  return {
    title: '',
    description: '',
    status: 'draft',
    slug: '',
    steps: [
      {
        id: 'step-1',
        title: '第一部分',
        description: '',
        questions: [blankQuestion('q1')],
      },
    ],
  };
}

function reset(next: Draft) {
  form.title = next.title;
  form.description = next.description;
  form.status = next.status;
  form.slug = next.slug;
  form.steps = next.steps;
}

async function load() {
  if (!isEdit.value) {
    reset(blankForm());
    responseCount.value = 0;
    return;
  }
  loading.value = true;
  loadError.value = '';
  try {
    const detail = await api<QuestionnaireDetail>(`/api/admin/questionnaires/${numericId.value}`);
    if (detail.undecodable || !detail.definition) {
      throw new Error(detail.error || '问卷定义无法解码,无法编辑');
    }
    const def: Definition = detail.definition;
    reset({
      title: def.title,
      description: def.description ?? '',
      status: detail.status,
      slug: detail.slug,
      steps: (def.steps ?? []).map((s, si) => ({
        id: s.id || `step-${si + 1}`,
        title: s.title,
        description: s.description ?? '',
        questions: (s.questions ?? []).map(normalizeQuestion),
      })),
    });
    // The detail payload has no response count, and the warning about orphaning
    // answers is worth one extra request.
    try {
      const list = await api<{ items: QuestionnaireSummary[] }>('/api/admin/questionnaires');
      responseCount.value = list.items.find((q) => q.id === numericId.value)?.responseCount ?? 0;
    } catch {
      /* the warning is advisory; a failure here must not block editing */
    }
  } catch (err) {
    loadError.value = errorMessage(err);
  } finally {
    loading.value = false;
  }
}

onMounted(load);

/**
 * One component serves both routes, so navigating from one questionnaire's
 * editor to another (or from a successful create into the editor) reuses this
 * instance. Watching the param is what keeps the form from showing stale data.
 */
watch(idParam, () => {
  created.value = null;
  savedAt.value = '';
  issue.value = null;
  void load();
});

// ---------------------------------------------------------------- editing ---

function moveIn<T>(list: T[], index: number, delta: number) {
  const next = index + delta;
  if (next < 0 || next >= list.length) return;
  const [item] = list.splice(index, 1);
  list.splice(next, 0, item);
}

function addStep() {
  form.steps.push({ id: nextStepId(), title: '', description: '', questions: [] });
}

function removeStep(index: number) {
  if (form.steps.length <= 1) {
    toast.info('至少需要保留一个步骤');
    return;
  }
  form.steps.splice(index, 1);
}

function addQuestion(step: DraftStep) {
  step.questions.push(blankQuestion(nextKey()));
}

function removeQuestion(step: DraftStep, index: number) {
  step.questions.splice(index, 1);
}

function onTypeChange(q: Question) {
  // Switching type leaves the fields of the old type behind, which would be
  // sent to the server and rejected by its strict validation.
  if (isChoice(q.type)) {
    q.options = q.options?.length ? q.options : defaultOptions();
    delete q.minLength;
    delete q.maxLength;
    delete q.min;
    delete q.max;
    delete q.maxImages;
  } else {
    delete q.options;
    if (q.type === 'rating') {
      q.min = q.min ?? 1;
      q.max = q.max ?? 5;
    }
    if (q.type === 'image') q.maxImages = q.maxImages ?? 1;
  }
}

function addOption(q: Question) {
  (q.options ??= []).push({ value: '', label: '' });
}

function removeOption(q: Question, index: number) {
  q.options?.splice(index, 1);
}

/** Number inputs emit strings; empty means "unset", not 0. */
function toNumber(value: string): number | undefined {
  const n = Number(value);
  return value.trim() === '' || Number.isNaN(n) ? undefined : n;
}

// -------------------------------------------------------------- validation ---

const KEY_RE = /^[A-Za-z0-9_-]{1,64}$/;

/**
 * Mirrors schema.Definition.Validate on the server, in the same order, so the
 * admin hears about a problem here rather than after a round trip. Returns the
 * FIRST issue: fixing them one at a time matches how the form is laid out.
 */
function firstIssue(): ValidationIssue | null {
  if (!form.title.trim()) return { message: '请填写问卷标题', stepIndex: -1 };
  if (form.title.trim().length > 200) return { message: '标题过长(最多 200 字)', stepIndex: -1 };
  if (form.steps.length === 0) return { message: '至少需要一个步骤', stepIndex: -1 };
  if (form.steps.length > 50) return { message: '步骤过多(最多 50 个)', stepIndex: -1 };

  const seen = new Set<string>();
  for (let si = 0; si < form.steps.length; si += 1) {
    const step = form.steps[si];
    if (!step.title.trim()) {
      return { message: `第 ${si + 1} 个步骤缺少标题`, stepIndex: si };
    }
    if (step.questions.length === 0) {
      return { message: `步骤「${step.title}」至少需要一道题`, stepIndex: si };
    }
    for (let qi = 0; qi < step.questions.length; qi += 1) {
      const q = step.questions[qi];
      const key = q.key.trim();
      if (!KEY_RE.test(key)) {
        return {
          message: `题目 ${qi + 1} 的标识非法(仅允许字母、数字、_ 和 -,最多 64 字符)`,
          stepIndex: si,
          questionIndex: qi,
        };
      }
      if (seen.has(key)) {
        return { message: `题目标识重复:${key}`, stepIndex: si, questionIndex: qi };
      }
      seen.add(key);
      if (!q.label.trim()) {
        return { message: `题目「${key}」缺少标题`, stepIndex: si, questionIndex: qi };
      }
      if (isChoice(q.type)) {
        const options = q.options ?? [];
        if (options.length < 2) {
          return { message: `题目「${key}」至少需要两个选项`, stepIndex: si, questionIndex: qi };
        }
        const values = new Set<string>();
        for (let oi = 0; oi < options.length; oi += 1) {
          const o = options[oi];
          if (!o.value.trim() || !o.label.trim()) {
            return {
              message: `题目「${key}」的第 ${oi + 1} 个选项不完整`,
              stepIndex: si,
              questionIndex: qi,
            };
          }
          if (values.has(o.value.trim())) {
            return {
              message: `题目「${key}」的选项值重复:${o.value.trim()}`,
              stepIndex: si,
              questionIndex: qi,
            };
          }
          values.add(o.value.trim());
        }
      }
      if (q.type === 'rating' && (q.min ?? 1) >= (q.max ?? 5)) {
        return { message: `题目「${key}」的评分区间非法`, stepIndex: si, questionIndex: qi };
      }
    }
  }
  if (totalQuestions.value > 500) return { message: '题目过多(最多 500 道)', stepIndex: -1 };
  return null;
}

/** Surfaces the issue and moves the viewport to the element it names. */
function showIssue(next: ValidationIssue) {
  issue.value = next;
  issueCount.value += 1;
  mobileTab.value = 'editor';
  nextTick(() => {
    const selector =
      next.questionIndex !== undefined
        ? `[data-q="${next.stepIndex}-${next.questionIndex}"]`
        : next.stepIndex >= 0
          ? `[data-step="${next.stepIndex}"]`
          : '[data-field="title"]';
    const el = document.querySelector<HTMLElement>(selector);
    el?.scrollIntoView({ behavior: 'smooth', block: 'center' });
    el?.querySelector<HTMLElement>('input, textarea')?.focus();
  });
}

function issueAt(si: number, qi?: number): ValidationIssue | null {
  const current = issue.value;
  if (!current) return null;
  if (current.stepIndex !== si) return null;
  if (qi === undefined) return current.questionIndex === undefined ? current : null;
  return current.questionIndex === qi ? current : null;
}

// ------------------------------------------------------------------- save ---

/** Strips editor-only noise: empty optional strings are omitted, not sent. */
function cleanQuestion(q: Question): Question {
  const out: Question = {
    key: q.key.trim(),
    type: q.type,
    label: q.label.trim(),
    required: !!q.required,
  };
  const help = (q.help ?? '').trim();
  if (help) out.help = help;
  if (isChoice(q.type)) {
    out.options = (q.options ?? []).map((o) => ({ value: o.value.trim(), label: o.label.trim() }));
  }
  if (q.type === 'text' || q.type === 'textarea') {
    if (q.minLength) out.minLength = q.minLength;
    if (q.maxLength) out.maxLength = q.maxLength;
  }
  if (q.type === 'rating') {
    out.min = q.min ?? 1;
    out.max = q.max ?? 5;
  }
  if (q.type === 'image') out.maxImages = q.maxImages ?? 1;
  return out;
}

function buildDefinition(): Definition {
  const def: Definition = {
    title: form.title.trim(),
    steps: form.steps.map((s, si) => ({
      id: s.id || `step-${si + 1}`,
      title: s.title.trim(),
      questions: s.questions.map(cleanQuestion),
    })),
  };
  const description = form.description.trim();
  if (description) def.description = description;
  const stepDescriptions = form.steps.map((s) => s.description.trim());
  stepDescriptions.forEach((d, i) => {
    if (d) def.steps[i].description = d;
  });
  return def;
}

function requestSave() {
  const found = firstIssue();
  if (found) {
    showIssue(found);
    return;
  }
  issue.value = null;
  // Re-encoding a definition cannot rewrite answers that already exist, so the
  // answers to a changed question are simply unreachable afterwards. Warn, and
  // let the admin decide.
  if (isEdit.value && responseCount.value > 0) {
    confirmOpen.value = true;
    return;
  }
  void save();
}

async function save() {
  saving.value = true;
  confirmOpen.value = false;
  try {
    const definition = buildDefinition();
    if (isEdit.value) {
      await api(`/api/admin/questionnaires/${numericId.value}`, {
        method: 'PUT',
        body: { status: form.status, definition },
      });
      savedAt.value = new Date().toISOString();
      toast.success('已保存', '修改将在下一次编码时生效');
    } else {
      const body: Record<string, unknown> = { status: form.status, definition };
      const slug = form.slug.trim();
      if (slug) body.slug = slug;
      const detail = await api<QuestionnaireDetail>('/api/admin/questionnaires', {
        method: 'POST',
        body,
      });
      created.value = { id: detail.id, slug: detail.slug };
      toast.success('问卷已创建', '把下面的链接发给受访者即可开始收集');
    }
  } catch (err) {
    toastError(err, '保存失败');
  } finally {
    saving.value = false;
  }
}

const shareUrl = computed(() =>
  created.value ? `${window.location.origin}/q/${created.value.slug}` : '',
);

async function copyShareLink() {
  if (!shareUrl.value) return;
  try {
    await navigator.clipboard.writeText(shareUrl.value);
    copied.value = true;
    toast.success('链接已复制');
    window.setTimeout(() => (copied.value = false), 2000);
  } catch {
    toast.error('复制失败', '请手动选中链接复制');
  }
}

function continueEditing() {
  if (!created.value) return;
  const id = created.value.id;
  // Clearing the panel first hands the page back to the editor; the route
  // watcher then reloads the definition the server just stored.
  created.value = null;
  savedAt.value = '';
  void router.replace({ name: 'admin-questionnaire-edit', params: { id } });
}

function startAnother() {
  // A create keeps the URL at /new, so the route watcher would not fire here.
  created.value = null;
  reset(blankForm());
  responseCount.value = 0;
  savedAt.value = '';
  void router.replace({ name: 'admin-questionnaire-new' });
}

// ---------------------------------------------------------------- preview ---

/** Unfilled shows the control as a respondent first meets it, empty. */
function previewValue(q: Question): unknown {
  if (!previewFilled.value) {
    if (q.type === 'multi' || q.type === 'image') return [];
    if (q.type === 'rating') return 0;
    return '';
  }
  switch (q.type) {
    case 'single':
    case 'select':
      return q.options?.[0]?.value ?? '';
    case 'multi':
      return (q.options ?? []).slice(0, 2).map((o) => o.value);
    case 'rating':
      return Math.ceil(((q.min ?? 1) + (q.max ?? 5)) / 2);
    case 'date':
      return new Date().toISOString().slice(0, 10);
    default:
      return '示例答案';
  }
}

const previewSteps = computed(() => form.steps);
const activeStep = ref(0);
const previewStep = computed<DraftStep | undefined>(() => form.steps[activeStep.value] ?? form.steps[0]);

/** The props a control needs, per type. Read-only: nothing here is committed. */
function previewAttrs(q: Question): Record<string, unknown> {
  const value = previewValue(q);
  const label = q.label || '(未命名题目)';
  const help = q.help?.trim() || undefined;
  const readOnly = { modelValue: value, disabled: true, 'onUpdate:modelValue': () => {} };
  switch (q.type) {
    case 'single':
    case 'multi':
      return { ...readOnly, options: q.options ?? [] };
    case 'rating':
      return { modelValue: value, readonly: true, min: q.min ?? 1, max: q.max ?? 5 };
    case 'select':
      return { ...readOnly, label, help, options: q.options ?? [] };
    case 'text':
      return { ...readOnly, label, help, maxlength: q.maxLength };
    case 'textarea':
      return { ...readOnly, label, help, maxlength: q.maxLength, rows: 3 };
    case 'date':
      return { ...readOnly, label, help };
    default:
      return readOnly;
  }
}
</script>

<template>
  <div class="mx-auto w-full max-w-7xl">
    <!-- ------------------------------------------------------- header --- -->
    <header class="mb-5 flex flex-wrap items-start justify-between gap-3">
      <!-- The layout already prints the page title from the route; repeating it
           here would just be a second copy of the same words. -->
      <div class="flex min-w-0 flex-wrap items-center gap-2">
        <StatusPill v-if="isEdit" :status="form.status" size="sm" />
        <p class="text-sm text-muted">
          定义以数独密文存储,此处的标题与题目是解码后的明文,仅管理员可见。
        </p>
      </div>
      <div class="flex items-center gap-2">
        <AppButton variant="ghost" @click="router.back()">返回</AppButton>
        <AppButton :loading="saving" @click="requestSave">
          <Save class="size-4" />
          保存
        </AppButton>
      </div>
    </header>

    <!-- ----------------------------------------------------- load states -->
    <div v-if="loading" class="space-y-4">
      <AppSkeleton variant="card" :lines="3" />
      <AppSkeleton variant="card" :lines="6" />
    </div>

    <AppCard v-else-if="loadError" padding="lg">
      <EmptyState
        :icon="AlertTriangle"
        title="无法加载问卷"
        :description="loadError"
      >
        <AppButton variant="secondary" @click="load">重试</AppButton>
      </EmptyState>
    </AppCard>

    <!-- ------------------------------------------------ create success --- -->
    <AppCard v-else-if="created" padding="lg" class="animate-fade-up">
      <div class="flex flex-col gap-6 sm:flex-row sm:items-start">
        <div class="min-w-0 flex-1">
          <div class="flex items-center gap-2 text-emerald-600 dark:text-emerald-400">
            <Check class="size-5" />
            <h2 class="text-lg font-semibold">问卷已创建</h2>
          </div>
          <p class="mt-2 text-sm text-muted">
            链接已经可用。发布状态为「草稿」时受访者无法填写,请在下方或问卷管理页切换为「已发布」。
          </p>

          <div class="mt-4">
            <p class="mb-1.5 text-xs font-medium text-subtle">填写链接</p>
            <div class="flex items-center gap-2">
              <code
                class="cipher min-w-0 flex-1 truncate rounded-[var(--radius-control)] border border-line bg-sunken px-3 py-2 text-ink"
              >{{ shareUrl }}</code>
              <AppButton variant="secondary" @click="copyShareLink">
                <component :is="copied ? Check : Copy" class="size-4" />
                复制
              </AppButton>
            </div>
          </div>

          <div class="mt-5 flex flex-wrap gap-2">
            <AppButton
              variant="secondary"
              @click="router.push({ name: 'admin-responses', params: { id: created.id } })"
            >
              <ListChecks class="size-4" />
              查看答卷
            </AppButton>
            <AppButton variant="ghost" @click="continueEditing">继续编辑</AppButton>
            <AppButton variant="ghost" @click="startAnother">再建一份</AppButton>
          </div>
        </div>

        <div class="shrink-0 self-center rounded-2xl border border-line bg-raised p-3">
          <QrCode :value="shareUrl" :size="180" label="扫码填写" />
        </div>
      </div>
    </AppCard>

    <!-- --------------------------------------------------------- editor --- -->
    <template v-else>
      <!-- Below lg the two panels become tabs so the form stays usable. -->
      <div class="mb-4 grid grid-cols-2 gap-1 rounded-[var(--radius-control)] border border-line bg-sunken p-1 lg:hidden">
        <button
          v-for="tab in (['editor', 'preview'] as const)"
          :key="tab"
          type="button"
          class="rounded-lg py-2 text-sm font-medium transition-colors"
          :class="mobileTab === tab ? 'bg-raised text-ink shadow-sm' : 'text-muted'"
          @click="mobileTab = tab"
        >
          {{ tab === 'editor' ? '编辑' : '预览' }}
        </button>
      </div>

      <div class="grid items-start gap-5 lg:grid-cols-[minmax(0,1fr)_380px]">
        <!-- ============================================== editor column -->
        <div :class="mobileTab === 'preview' ? 'hidden lg:block' : 'block'">
          <AppCard v-if="responseCount > 0" padding="sm" class="mb-4 border-amber-300 bg-amber-50/60 dark:border-amber-500/30 dark:bg-amber-500/5">
            <div class="flex items-start gap-3">
              <AlertTriangle class="mt-0.5 size-4 shrink-0 text-amber-600 dark:text-amber-400" />
              <p class="text-sm text-amber-800 dark:text-amber-200">
                该问卷已有 <span class="font-semibold tabular-nums">{{ responseCount }}</span> 份答卷。
                修改题目(尤其是删除题目或改动选项值)会让这些答卷的部分答案再也无法对应,已有的数据不会丢失,
                但新定义下可能显示为空。仍可保存。
              </p>
            </div>
          </AppCard>

          <!-- first error, loudly -->
          <AppCard
            v-if="issue"
            :key="issueCount"
            padding="sm"
            class="mb-4 border-rose-300 bg-rose-50/70 animate-fade-up dark:border-rose-500/30 dark:bg-rose-500/5"
          >
            <div class="flex items-start gap-3">
              <AlertTriangle class="mt-0.5 size-4 shrink-0 text-rose-600 dark:text-rose-400" />
              <p class="text-sm text-rose-800 dark:text-rose-200">{{ issue.message }}</p>
            </div>
          </AppCard>

          <!-- questionnaire basics -->
          <AppCard class="mb-4">
            <div data-field="title">
              <AppInput
                v-model="form.title"
                label="问卷标题"
                placeholder="例如:员工满意度调查"
                required
                :maxlength="200"
              />
            </div>
            <div class="mt-4">
              <AppTextarea
                v-model="form.description"
                label="问卷说明"
                :rows="2"
                placeholder="向受访者说明调查目的(可留空)"
                :maxlength="1000"
                show-count
              />
            </div>
            <div class="mt-4 grid gap-4 sm:grid-cols-2">
              <AppSelect v-model="form.status" label="状态" :options="STATUS_OPTIONS" />
              <AppInput
                v-if="!isEdit"
                v-model="form.slug"
                label="自定义链接(可留空)"
                placeholder="自动生成"
                help="仅小写字母、数字和连字符"
                prefix="/q/"
              />
            </div>
          </AppCard>

          <!-- steps -->
          <div class="stagger space-y-4">
            <AppCard
              v-for="(step, si) in form.steps"
              :key="step.id"
              :data-step="si"
              :style="{ '--i': si }"
            >
              <template #header>
                <div class="flex items-center gap-2">
                  <span
                    class="grid size-6 shrink-0 place-items-center rounded-md bg-brand-50 text-xs font-semibold text-brand-700 dark:bg-brand-500/15 dark:text-brand-300"
                  >
                    {{ si + 1 }}
                  </span>
                  <span class="text-sm font-medium text-muted">
                    步骤 · {{ step.questions.length }} 题
                  </span>
                </div>
              </template>
              <template #actions>
                <div class="flex items-center gap-1">
                  <AppButton
                    variant="ghost"
                    size="sm"
                    :disabled="si === 0"
                    title="上移"
                    @click="moveIn(form.steps, si, -1)"
                  >
                    <ArrowUp class="size-4" />
                  </AppButton>
                  <AppButton
                    variant="ghost"
                    size="sm"
                    :disabled="si === form.steps.length - 1"
                    title="下移"
                    @click="moveIn(form.steps, si, 1)"
                  >
                    <ArrowDown class="size-4" />
                  </AppButton>
                  <AppButton
                    variant="ghost"
                    size="sm"
                    title="删除步骤"
                    :disabled="form.steps.length <= 1"
                    @click="removeStep(si)"
                  >
                    <Trash2 class="size-4" />
                  </AppButton>
                </div>
              </template>

              <div class="grid gap-3">
                <AppInput v-model="step.title" label="步骤标题" placeholder="例如:基本信息" />
                <AppInput v-model="step.description" label="步骤说明(可留空)" />
              </div>
              <p
                v-if="issueAt(si) && issueAt(si)?.questionIndex === undefined"
                class="mt-2 text-xs text-rose-600 dark:text-rose-400"
              >
                {{ issueAt(si)?.message }}
              </p>

              <!-- questions -->
              <div class="mt-4 space-y-3">
                <div
                  v-for="(q, qi) in step.questions"
                  :key="`${step.id}-${qi}`"
                  :data-q="`${si}-${qi}`"
                  class="rounded-[var(--radius-control)] border bg-sunken/60 p-3 sm:p-4"
                  :class="
                    issueAt(si, qi) ? 'border-rose-400 dark:border-rose-500/60' : 'border-line'
                  "
                >
                  <div class="mb-3 flex items-center justify-between gap-2">
                    <span class="font-mono text-xs text-subtle tabular-nums">
                      #{{ qi + 1 }} · {{ typeLabel(q.type) }}
                    </span>
                    <div class="flex items-center gap-1">
                      <AppButton
                        variant="ghost"
                        size="sm"
                        :disabled="qi === 0"
                        title="上移"
                        @click="moveIn(step.questions, qi, -1)"
                      >
                        <ArrowUp class="size-3.5" />
                      </AppButton>
                      <AppButton
                        variant="ghost"
                        size="sm"
                        :disabled="qi === step.questions.length - 1"
                        title="下移"
                        @click="moveIn(step.questions, qi, 1)"
                      >
                        <ArrowDown class="size-3.5" />
                      </AppButton>
                      <AppButton variant="ghost" size="sm" title="删除题目" @click="removeQuestion(step, qi)">
                        <Trash2 class="size-3.5" />
                      </AppButton>
                    </div>
                  </div>

                  <div class="grid gap-3 sm:grid-cols-2">
                    <AppInput
                      v-model="q.key"
                      label="标识"
                      class="font-mono"
                      placeholder="q1"
                      help="字母、数字、_ 和 -,导出时的列名"
                    />
                    <AppSelect
                      :model-value="q.type"
                      label="题型"
                      :options="TYPE_OPTIONS"
                      @update:model-value="
                        (v: string) => {
                          q.type = v as QuestionType;
                          onTypeChange(q);
                        }
                      "
                    />
                  </div>

                  <div class="mt-3 grid gap-3">
                    <AppInput v-model="q.label" label="题目" placeholder="你想问什么?" required />
                    <AppInput v-model="q.help" label="补充说明(可留空)" />
                  </div>

                  <!-- required -->
                  <label class="mt-3 flex cursor-pointer items-center gap-2 text-sm text-muted">
                    <input
                      v-model="q.required"
                      type="checkbox"
                      class="size-4 rounded border-line text-brand-600 focus:ring-brand-500"
                    />
                    必答
                  </label>

                  <!-- type-specific -->
                  <div v-if="isChoice(q.type)" class="mt-3">
                    <p class="mb-2 text-xs font-medium text-subtle">选项</p>
                    <div class="space-y-2">
                      <div v-for="(opt, oi) in q.options ?? []" :key="oi" class="flex items-center gap-2">
                        <AppInput
                          v-model="opt.value"
                          placeholder="值"
                          class="w-28 shrink-0 sm:w-32"
                        />
                        <AppInput v-model="opt.label" placeholder="显示文字" />
                        <AppButton
                          variant="ghost"
                          size="sm"
                          title="删除选项"
                          @click="removeOption(q, oi)"
                        >
                          <Trash2 class="size-3.5" />
                        </AppButton>
                      </div>
                    </div>
                    <AppButton variant="ghost" size="sm" class="mt-2" @click="addOption(q)">
                      <Plus class="size-3.5" />
                      添加选项
                    </AppButton>
                  </div>

                  <div
                    v-else-if="q.type === 'text' || q.type === 'textarea'"
                    class="mt-3 grid gap-3 sm:grid-cols-2"
                  >
                    <AppInput
                      :model-value="q.minLength"
                      label="最少字数"
                      type="number"
                      inputmode="numeric"
                      @update:model-value="(v: string) => (q.minLength = toNumber(v))"
                    />
                    <AppInput
                      :model-value="q.maxLength"
                      label="最多字数"
                      type="number"
                      inputmode="numeric"
                      @update:model-value="(v: string) => (q.maxLength = toNumber(v))"
                    />
                  </div>

                  <div v-else-if="q.type === 'rating'" class="mt-3 grid gap-3 sm:grid-cols-2">
                    <AppInput
                      :model-value="q.min ?? 1"
                      label="最低分"
                      type="number"
                      inputmode="numeric"
                      @update:model-value="(v: string) => (q.min = toNumber(v) ?? 1)"
                    />
                    <AppInput
                      :model-value="q.max ?? 5"
                      label="最高分"
                      type="number"
                      inputmode="numeric"
                      @update:model-value="(v: string) => (q.max = toNumber(v) ?? 5)"
                    />
                  </div>

                  <div v-else-if="q.type === 'image'" class="mt-3">
                    <AppInput
                      :model-value="q.maxImages ?? 1"
                      label="最多上传张数"
                      type="number"
                      inputmode="numeric"
                      @update:model-value="(v: string) => (q.maxImages = toNumber(v) ?? 1)"
                    />
                  </div>

                  <p
                    v-if="issueAt(si, qi)"
                    class="mt-3 text-xs font-medium text-rose-600 dark:text-rose-400"
                  >
                    {{ issueAt(si, qi)?.message }}
                  </p>
                </div>
              </div>

              <AppButton variant="secondary" size="sm" class="mt-3" @click="addQuestion(step)">
                <Plus class="size-4" />
                添加题目
              </AppButton>
            </AppCard>
          </div>

          <AppButton variant="secondary" block class="mt-4" @click="addStep">
            <Plus class="size-4" />
            添加步骤
          </AppButton>

          <p class="mt-4 text-center text-xs text-subtle tabular-nums">
            共 {{ form.steps.length }} 个步骤 · {{ totalQuestions }} 道题
            <template v-if="savedAt"> · 已保存</template>
          </p>
        </div>

        <!-- ============================================= preview column -->
        <div
          :class="mobileTab === 'editor' ? 'hidden lg:block' : 'block'"
          class="lg:sticky lg:top-4"
        >
          <AppCard padding="none" class="overflow-hidden">
            <div class="flex items-center justify-between gap-2 border-b border-line px-4 py-3">
              <div class="flex items-center gap-2">
                <Eye class="size-4 text-brand-500" />
                <h2 class="text-sm font-semibold text-ink">实时预览</h2>
              </div>
              <button
                type="button"
                class="flex items-center gap-1 text-xs text-muted transition-colors hover:text-ink"
                @click="previewFilled = !previewFilled"
              >
                <component :is="previewFilled ? EyeOff : Eye" class="size-3.5" />
                {{ previewFilled ? '看空态' : '填示例' }}
              </button>
            </div>

            <div
              v-if="previewSteps.length > 1"
              class="flex flex-wrap gap-1.5 border-b border-line bg-sunken/50 px-3 py-2"
            >
              <button
                v-for="(s, i) in previewSteps"
                :key="s.id"
                type="button"
                class="rounded-full px-2.5 py-1 text-xs font-medium transition-colors"
                :class="
                  activeStep === i
                    ? 'bg-brand-600 text-white dark:bg-brand-500'
                    : 'bg-raised text-muted hover:text-ink'
                "
                @click="activeStep = i"
              >
                {{ s.title || `步骤 ${i + 1}` }}
              </button>
            </div>

            <div class="max-h-[62vh] overflow-y-auto p-4">
              <template v-if="previewStep">
                <h3 class="text-base font-semibold text-ink">
                  {{ previewStep.title || '未命名步骤' }}
                </h3>
                <p v-if="previewStep.description" class="mt-1 text-sm text-muted">
                  {{ previewStep.description }}
                </p>

                <div v-if="previewStep.questions.length === 0" class="py-8">
                  <EmptyState title="这一步还没有题目" description="在左侧添加题目后会立刻出现在这里。" />
                </div>

                <div v-else class="mt-4 space-y-5">
                  <div v-for="(q, qi) in previewStep.questions" :key="qi">
                    <!-- radio / checkbox / rating draw no label of their own -->
                    <div v-if="!OWNS_LABEL.includes(q.type)" class="mb-1.5">
                      <p class="text-sm font-medium text-ink">
                        {{ q.label || '(未命名题目)' }}
                        <span v-if="q.required" class="ml-0.5 text-rose-500" aria-hidden="true">*</span>
                      </p>
                      <p v-if="q.help" class="mt-0.5 text-xs text-subtle">{{ q.help }}</p>
                    </div>

                    <!-- the upload control needs a live upload endpoint that an
                         admin preview does not have, so it is stood in for -->
                    <div v-if="q.type === 'image'" class="mt-2 grid grid-cols-3 gap-2.5 sm:grid-cols-4">
                      <div
                        v-for="n in Math.min(q.maxImages ?? 1, 4)"
                        :key="n"
                        class="grid aspect-square place-content-center justify-items-center gap-1.5 rounded-[var(--radius-control)] border border-dashed border-line-strong bg-raised text-subtle opacity-70"
                      >
                        <ImageIcon class="size-5" aria-hidden="true" />
                        <span class="text-[11px]">添加图片</span>
                      </div>
                    </div>
                    <component v-else :is="CONTROLS[q.type]" v-bind="previewAttrs(q)" />
                  </div>
                </div>

                <p class="mt-6 text-center text-xs text-subtle">
                  预览为只读,受访者看到的就是这些组件。
                </p>
              </template>
            </div>
          </AppCard>
        </div>
      </div>
    </template>

    <AppConfirm
      :open="confirmOpen"
      title="该问卷已有答卷"
      :message="`已有 ${responseCount} 份答卷。修改题目定义可能让这些答卷的答案无法对应,是否仍要保存?`"
      confirm-text="仍要保存"
      cancel-text="取消"
      danger
      :loading="saving"
      @confirm="save"
      @cancel="confirmOpen = false"
    />
  </div>
</template>
