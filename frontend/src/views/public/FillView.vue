<script setup lang="ts">
/**
 * The respondent's form.
 *
 * Order of operations matters here and is the whole point of the page:
 *
 *   1. fetch the CIPHERTEXT definition plus the derived key and its fingerprint;
 *   2. verify the browser derived the same table as the server (`prepare`);
 *   3. only then decode the definition, locally.
 *
 * Step 2 before step 3 is deliberate. Decoding with a table the server does not
 * share yields plausible-looking garbage rather than an error, so a mismatch has
 * to be caught while there is still nothing to lose.
 */
import { AlertTriangle, ArrowLeft, ArrowRight, Lock, Send } from 'lucide-vue-next';
import { computed, onMounted, ref, watch } from 'vue';
import { RouterLink, useRoute, useRouter } from 'vue-router';

import { ApiError, api } from '@/api/client';
import type { AnswerValue, Definition, PublicQuestionnaire, Question, Step } from '@/api/types';
import AppButton from '@/components/ui/AppButton.vue';
import AppCard from '@/components/ui/AppCard.vue';
import AppCheckboxGroup from '@/components/ui/AppCheckboxGroup.vue';
import AppDateField from '@/components/ui/AppDateField.vue';
import AppImageUpload from '@/components/ui/AppImageUpload.vue';
import AppInput from '@/components/ui/AppInput.vue';
import AppRadioGroup from '@/components/ui/AppRadioGroup.vue';
import AppRating from '@/components/ui/AppRating.vue';
import AppSelect from '@/components/ui/AppSelect.vue';
import AppSpinner from '@/components/ui/AppSpinner.vue';
import AppTextarea from '@/components/ui/AppTextarea.vue';
import EmptyState from '@/components/ui/EmptyState.vue';
import SudokuMark from '@/components/ui/SudokuMark.vue';
import ThemeToggle from '@/components/ui/ThemeToggle.vue';
import { parseLayoutId, textToBytes, utf8Encode, type LayoutId } from '@/codec';
import { errorMessage, toast, toastError } from '@/lib/toast';
import { useCodecStore } from '@/stores/codec';

const route = useRoute();
const router = useRouter();
const codec = useCodecStore();

const slug = computed(() => String(route.params.slug ?? ''));

type Status = 'loading' | 'ready' | 'failed';
const status = ref<Status>('loading');
const failure = ref({ title: '', detail: '', retry: true });

const definition = ref<Definition | null>(null);
const derivedKey = ref('');
const layoutId = ref<LayoutId>('ascii');

const answers = ref<Record<string, AnswerValue>>({});
const errors = ref<Record<string, string>>({});
const stepIndex = ref(0);
const direction = ref<'next' | 'prev'>('next');
const submitting = ref(false);

/** Wall-clock from the moment the form opened, reported as `durationMs`. */
const mountedAt = ref(Date.now());

const steps = computed<Step[]>(() => definition.value?.steps ?? []);
const currentStep = computed<Step | null>(() => steps.value[stepIndex.value] ?? null);
const isLast = computed(() => stepIndex.value >= steps.value.length - 1);
const progressPercent = computed(() =>
  steps.value.length ? Math.round(((stepIndex.value + 1) / steps.value.length) * 100) : 0,
);
const transitionName = computed(() => (direction.value === 'next' ? 'slide-next' : 'slide-prev'));

// ---------------------------------------------------------------------------
// Loading
// ---------------------------------------------------------------------------

function emptyAnswer(q: Question): AnswerValue {
  if (q.type === 'multi' || q.type === 'image') return [];
  if (q.type === 'rating') return null;
  return '';
}

function initAnswers(def: Definition): void {
  const next: Record<string, AnswerValue> = {};
  for (const step of def.steps ?? []) {
    for (const q of step.questions ?? []) next[q.key] = emptyAnswer(q);
  }
  answers.value = next;
  errors.value = {};
}

async function load(): Promise<void> {
  status.value = 'loading';
  definition.value = null;
  errors.value = {};
  stepIndex.value = 0;

  try {
    const meta = await api<PublicQuestionnaire>(
      `/api/public/questionnaires/${encodeURIComponent(slug.value)}`,
      { auth: false },
    );
    layoutId.value = parseLayoutId(meta.layout);

    // Throws with a readable message when this browser and the server do not
    // agree on the coding table.
    await codec.prepare(meta.derivedKey, layoutId.value, meta.fingerprint);

    let def: Definition;
    try {
      def = await codec.decodeJson<Definition>(meta.schemaCipher, meta.derivedKey, layoutId.value);
    } catch (err) {
      throw new Error(`浏览器与服务器的编码表未能达成一致,问卷结构无法解读(${errorMessage(err)})`);
    }
    if (!def || !Array.isArray(def.steps) || def.steps.length === 0) {
      throw new Error('问卷结构为空,请联系发布者重新保存');
    }

    derivedKey.value = meta.derivedKey;
    definition.value = def;
    initAnswers(def);
    if (def.title) document.title = `${def.title} · QRforQA`;
    status.value = 'ready';
  } catch (err) {
    if (err instanceof ApiError && (err.status === 403 || err.status === 404)) {
      failure.value =
        err.status === 403
          ? {
              title: '该问卷暂未发布或已关闭',
              detail: '发布者可能暂停了收集,或这份问卷已经结束。可以稍后再来,或联系发布者。',
              retry: false,
            }
          : {
              title: '问卷不存在',
              detail: '这个链接对应的问卷没有找到,可能是链接有误,或者问卷已被删除。',
              retry: false,
            };
    } else {
      failure.value = { title: '无法打开问卷', detail: errorMessage(err), retry: true };
    }
    status.value = 'failed';
  }
}

// ---------------------------------------------------------------------------
// Answer access
//
// Answers live in one loosely-typed record because the shape depends on the
// question type; these accessors narrow it for the controls and keep the
// template free of casts.
// ---------------------------------------------------------------------------

function text(key: string): string {
  const v = answers.value[key];
  if (typeof v === 'string') return v;
  if (typeof v === 'number') return String(v);
  return '';
}

function list(key: string): string[] {
  const v = answers.value[key];
  return Array.isArray(v) ? (v as unknown[]).map((x) => String(x)) : [];
}

function images(key: string): number[] {
  const v = answers.value[key];
  if (!Array.isArray(v)) return [];
  return (v as unknown[]).map((x) => Number(x)).filter((n) => Number.isFinite(n));
}

function ratingOf(key: string): number | null {
  const v = answers.value[key];
  return typeof v === 'number' ? v : null;
}

/** Coerces whatever a control emitted into our answer shape. */
function coerce(value: unknown): AnswerValue {
  if (value === null || value === undefined) return null;
  if (Array.isArray(value)) {
    return value.every((v) => typeof v === 'number')
      ? (value as number[])
      : value.map((v) => String(v));
  }
  if (typeof value === 'number' || typeof value === 'string') return value;
  if (typeof value === 'boolean') return value ? 'true' : '';
  return String(value);
}

function setAnswer(key: string, value: unknown): void {
  answers.value[key] = coerce(value);
  // An inline error is stale the moment the respondent edits the field.
  if (errors.value[key]) {
    const next = { ...errors.value };
    delete next[key];
    errors.value = next;
  }
}

// ---------------------------------------------------------------------------
// Validation
// ---------------------------------------------------------------------------

function isBlank(value: AnswerValue | undefined): boolean {
  if (value === null || value === undefined) return true;
  if (typeof value === 'string') return value.trim() === '';
  if (Array.isArray(value)) return value.length === 0;
  return false;
}

function validateQuestion(q: Question, value: AnswerValue | undefined): string {
  if (isBlank(value)) return q.required ? '此项为必填' : '';
  if (q.type === 'text' || q.type === 'textarea') {
    const raw = typeof value === 'string' ? value : String(value);
    if (q.minLength && raw.trim().length < q.minLength) return `至少需要 ${q.minLength} 个字符`;
    if (q.maxLength && raw.length > q.maxLength) return `最多 ${q.maxLength} 个字符`;
  }
  if (q.type === 'rating' && typeof value === 'number') {
    const min = q.min ?? 1;
    const max = q.max ?? 5;
    if (value < min || value > max) return `评分需要在 ${min} 到 ${max} 之间`;
  }
  return '';
}

function validateStep(index: number): boolean {
  const step = steps.value[index];
  if (!step) return true;
  const next = { ...errors.value };
  let ok = true;
  for (const q of step.questions ?? []) {
    const message = validateQuestion(q, answers.value[q.key]);
    if (message) {
      next[q.key] = message;
      ok = false;
    } else {
      delete next[q.key];
    }
  }
  errors.value = next;
  return ok;
}

/** Validates everything and reports the FIRST step that has a problem. */
function validateAll(): number {
  const next = { ...errors.value };
  let firstBad = -1;
  steps.value.forEach((step, index) => {
    let bad = false;
    for (const q of step.questions ?? []) {
      const message = validateQuestion(q, answers.value[q.key]);
      if (message) {
        next[q.key] = message;
        bad = true;
      } else {
        delete next[q.key];
      }
    }
    if (bad && firstBad === -1) firstBad = index;
  });
  errors.value = next;
  return firstBad;
}

// ---------------------------------------------------------------------------
// Navigation
// ---------------------------------------------------------------------------

function goTo(index: number, dir: 'next' | 'prev'): void {
  direction.value = dir;
  stepIndex.value = index;
  window.scrollTo({ top: 0, behavior: 'smooth' });
}

function goNext(): void {
  if (!validateStep(stepIndex.value)) {
    toast.error('还有题目没有完成', '请检查本步中标红的题目');
    return;
  }
  if (isLast.value) {
    void submit();
    return;
  }
  goTo(stepIndex.value + 1, 'next');
}

function goPrev(): void {
  if (stepIndex.value > 0) goTo(stepIndex.value - 1, 'prev');
}

// ---------------------------------------------------------------------------
// Images
// ---------------------------------------------------------------------------

/**
 * Encodes the base64 payload the upload control produced.
 *
 * The bytes encoded are the ASCII bytes of the base64 STRING, not the decoded
 * image: the server sudoku-decodes the ciphertext back into base64 text and then
 * runs `base64.StdEncoding.DecodeString` on it.
 */
async function codecEncode(base64: string): Promise<string> {
  return codec.encodeBytes(textToBytes(base64), derivedKey.value, layoutId.value);
}

/**
 * Uploads one image.
 *
 * The upload control is authored alongside this view, so the call shape is
 * accepted either way — a single metadata object, or the ciphertext first with
 * the metadata positionally. `fieldKey` always comes from the closure, so the
 * server can never be told the wrong question.
 */
function makeUploadFn(fieldKey: string) {
  return async (payload: any, ...rest: any[]): Promise<{ id: number }> => {
    const meta: Record<string, any> =
      payload && typeof payload === 'object' ? payload : { ...(rest[0] ?? {}), cipher: payload };
    return api<{ id: number }>(
      `/api/public/questionnaires/${encodeURIComponent(slug.value)}/uploads`,
      {
        method: 'POST',
        auth: false,
        body: {
          cipher: String(meta.cipher ?? ''),
          layout: layoutId.value,
          mime: String(meta.mime ?? 'application/octet-stream'),
          width: Number(meta.width ?? 0),
          height: Number(meta.height ?? 0),
          fieldKey,
        },
      },
    );
  };
}

// Built once per question so the control never sees a new function identity on
// re-render (which would restart an in-flight upload).
const uploadFns = new Map<string, ReturnType<typeof makeUploadFn>>();

function uploadFnFor(key: string) {
  let fn = uploadFns.get(key);
  if (!fn) {
    fn = makeUploadFn(key);
    uploadFns.set(key, fn);
  }
  return fn;
}

// ---------------------------------------------------------------------------
// Submit
// ---------------------------------------------------------------------------

async function submit(): Promise<void> {
  const firstBad = validateAll();
  if (firstBad >= 0) {
    toast.error(`第 ${firstBad + 1} 步还有题目没有完成`, '已跳到该步');
    goTo(firstBad, firstBad < stepIndex.value ? 'prev' : 'next');
    return;
  }

  submitting.value = true;
  try {
    const plain = JSON.stringify(answers.value);
    const cipher = await codec.encodeJson(answers.value, derivedKey.value, layoutId.value);
    const durationMs = Math.max(0, Date.now() - mountedAt.value);

    await api<{ id: number }>(
      `/api/public/questionnaires/${encodeURIComponent(slug.value)}/responses`,
      { method: 'POST', auth: false, body: { cipher, layout: layoutId.value, durationMs } },
    );

    // The byte counts are handed over in the query so the success page can show
    // them without a second round trip — and so a refresh still works.
    await router.replace({
      name: 'submitted',
      params: { slug: slug.value },
      query: {
        bytes: String(utf8Encode(plain).length),
        cipher: String(cipher.length),
        ms: String(durationMs),
      },
    });
  } catch (err) {
    // Answers are deliberately left untouched: a failed submit must never cost
    // the respondent their typing.
    submitting.value = false;
    toastError(err, '提交失败,答案已保留');
  }
}

onMounted(() => {
  mountedAt.value = Date.now();
  void load();
});

// Vue Router reuses this component across `/q/:slug` params, so a changed slug
// has to decode its own definition rather than keep showing the previous one.
watch(slug, (next, prev) => {
  if (next && next !== prev) void load();
});
</script>

<template>
  <div class="flex min-h-full flex-col bg-surface">
    <!-- Decoding: the bitmap is verified, the table derived, the schema decoded. -->
    <div
      v-if="status === 'loading'"
      class="flex flex-1 flex-col items-center justify-center gap-6 px-6 py-24 text-center"
    >
      <SudokuMark size="lg" />
      <div class="space-y-1.5">
        <p class="text-sm font-medium text-ink">正在解读问卷</p>
        <p class="font-mono text-[11px] text-subtle">校验位图 → 派生编码表 → 解出题目</p>
      </div>
      <AppSpinner />
    </div>

    <template v-else-if="status === 'failed'">
      <div class="mx-auto flex w-full max-w-lg flex-1 flex-col justify-center px-4 py-20 sm:px-6">
        <AppCard padding="none">
          <EmptyState
            :title="failure.title"
            :description="failure.detail"
            :icon="failure.retry ? AlertTriangle : Lock"
          >
            <div class="flex flex-wrap items-center justify-center gap-3">
              <AppButton v-if="failure.retry" variant="secondary" @click="load">
                重新加载
              </AppButton>
              <RouterLink to="/" class="text-sm text-muted underline-offset-4 hover:text-ink hover:underline">
                返回首页
              </RouterLink>
            </div>
          </EmptyState>
        </AppCard>
      </div>
    </template>

    <template v-else-if="definition && currentStep">
      <header class="glass sticky top-0 z-30 border-b border-line">
        <div class="mx-auto flex h-14 max-w-[720px] items-center gap-3 px-4 sm:px-6">
          <SudokuMark size="sm" :animate="false" pencil />
          <!-- The questionnaire is the document, so its title is the h1 and the
               steps are h2s. -->
          <h1 class="min-w-0 flex-1 truncate text-sm font-medium text-ink">
            {{ definition.title || '填写问卷' }}
          </h1>
          <ThemeToggle />
        </div>
      </header>

      <main class="mx-auto w-full max-w-[720px] flex-1 px-4 py-6 sm:px-6 sm:py-10">
        <p
          v-if="definition.description"
          class="mb-6 text-sm leading-6 text-muted text-balance"
        >
          {{ definition.description }}
        </p>

        <div
          class="mb-5"
          role="progressbar"
          :aria-valuenow="progressPercent"
          aria-valuemin="0"
          aria-valuemax="100"
        >
          <div class="mb-2 flex items-center justify-between text-xs">
            <span class="font-medium text-muted">第 {{ stepIndex + 1 }} / {{ steps.length }} 步</span>
            <span class="font-mono text-subtle tabular-nums">{{ progressPercent }}%</span>
          </div>
          <div class="h-1.5 w-full overflow-hidden rounded-full bg-sunken">
            <div
              class="h-full rounded-full bg-brand-600 transition-[width] duration-500 ease-out dark:bg-brand-500"
              :style="{ width: `${progressPercent}%` }"
            />
          </div>
        </div>

        <div class="steps-viewport">
          <Transition :name="transitionName">
            <div :key="stepIndex" class="min-w-0">
              <AppCard padding="lg">
                <template #title>
                  <h2 class="text-lg leading-7 font-semibold tracking-tight text-ink text-balance">
                    {{ currentStep.title || `第 ${stepIndex + 1} 步` }}
                  </h2>
                  <p
                    v-if="currentStep.description"
                    class="mt-1 text-sm leading-6 text-muted text-balance"
                  >
                    {{ currentStep.description }}
                  </p>
                </template>

                <div class="space-y-7">
                  <div v-for="q in currentStep.questions" :key="q.key" class="scroll-mt-28">
                    <!-- Controls that render their own label, help and error. -->
                    <AppSelect
                      v-if="q.type === 'select'"
                      :model-value="text(q.key)"
                      :label="q.label"
                      :help="q.help"
                      :error="errors[q.key]"
                      :required="q.required"
                      :disabled="submitting"
                      placeholder="请选择"
                      :options="q.options ?? []"
                      @update:model-value="setAnswer(q.key, $event)"
                    />

                    <AppInput
                      v-else-if="q.type === 'text'"
                      :model-value="text(q.key)"
                      :label="q.label"
                      :help="q.help"
                      :error="errors[q.key]"
                      :required="q.required"
                      :disabled="submitting"
                      :maxlength="q.maxLength"
                      :placeholder="q.required ? '请填写' : '选填'"
                      @update:model-value="setAnswer(q.key, $event)"
                    />

                    <AppTextarea
                      v-else-if="q.type === 'textarea'"
                      :model-value="text(q.key)"
                      :label="q.label"
                      :help="q.help"
                      :error="errors[q.key]"
                      :required="q.required"
                      :disabled="submitting"
                      :rows="4"
                      :maxlength="q.maxLength"
                      :placeholder="q.required ? '请填写' : '选填'"
                      @update:model-value="setAnswer(q.key, $event)"
                    />

                    <AppDateField
                      v-else-if="q.type === 'date'"
                      :model-value="text(q.key)"
                      :label="q.label"
                      :help="q.help"
                      :error="errors[q.key]"
                      :required="q.required"
                      :disabled="submitting"
                      @update:model-value="setAnswer(q.key, $event)"
                    />

                    <!-- The rest carry no label of their own, so it is rendered here. -->
                    <fieldset v-else-if="q.type === 'single'" class="min-w-0">
                      <legend class="flex items-center gap-1.5 text-sm font-medium text-ink">
                        {{ q.label }}
                        <span v-if="q.required" class="text-rose-500" aria-hidden="true">*</span>
                      </legend>
                      <p v-if="q.help" class="mt-1 text-xs text-subtle">{{ q.help }}</p>
                      <div class="mt-2.5">
                        <AppRadioGroup
                          :model-value="text(q.key)"
                          :options="q.options ?? []"
                          :name="q.key"
                          :columns="1"
                          :disabled="submitting"
                          @update:model-value="setAnswer(q.key, $event)"
                        />
                      </div>
                      <p
                        v-if="errors[q.key]"
                        class="mt-1.5 text-xs text-rose-600 dark:text-rose-400"
                      >
                        {{ errors[q.key] }}
                      </p>
                    </fieldset>

                    <fieldset v-else-if="q.type === 'multi'" class="min-w-0">
                      <legend class="flex items-center gap-1.5 text-sm font-medium text-ink">
                        {{ q.label }}
                        <span v-if="q.required" class="text-rose-500" aria-hidden="true">*</span>
                      </legend>
                      <p v-if="q.help" class="mt-1 text-xs text-subtle">{{ q.help }}</p>
                      <div class="mt-2.5">
                        <AppCheckboxGroup
                          :model-value="list(q.key)"
                          :options="q.options ?? []"
                          :columns="1"
                          :disabled="submitting"
                          @update:model-value="setAnswer(q.key, $event)"
                        />
                      </div>
                      <p
                        v-if="errors[q.key]"
                        class="mt-1.5 text-xs text-rose-600 dark:text-rose-400"
                      >
                        {{ errors[q.key] }}
                      </p>
                    </fieldset>

                    <fieldset v-else-if="q.type === 'rating'" class="min-w-0">
                      <legend class="flex items-center gap-1.5 text-sm font-medium text-ink">
                        {{ q.label }}
                        <span v-if="q.required" class="text-rose-500" aria-hidden="true">*</span>
                      </legend>
                      <p v-if="q.help" class="mt-1 text-xs text-subtle">{{ q.help }}</p>
                      <div class="mt-2.5">
                        <AppRating
                          :model-value="ratingOf(q.key)"
                          :min="q.min ?? 1"
                          :max="q.max ?? 5"
                          @update:model-value="setAnswer(q.key, $event)"
                        />
                      </div>
                      <p
                        v-if="errors[q.key]"
                        class="mt-1.5 text-xs text-rose-600 dark:text-rose-400"
                      >
                        {{ errors[q.key] }}
                      </p>
                    </fieldset>

                    <fieldset v-else-if="q.type === 'image'" class="min-w-0">
                      <legend class="flex items-center gap-1.5 text-sm font-medium text-ink">
                        {{ q.label }}
                        <span v-if="q.required" class="text-rose-500" aria-hidden="true">*</span>
                      </legend>
                      <p v-if="q.help" class="mt-1 text-xs text-subtle">{{ q.help }}</p>
                      <div class="mt-2.5">
                        <AppImageUpload
                          :model-value="images(q.key)"
                          :field-key="q.key"
                          :layout="layoutId"
                          :max-images="q.maxImages ?? 1"
                          :upload-fn="uploadFnFor(q.key)"
                          :codec-encode="codecEncode"
                          :disabled="submitting"
                          @update:model-value="setAnswer(q.key, $event)"
                        />
                      </div>
                      <p
                        v-if="errors[q.key]"
                        class="mt-1.5 text-xs text-rose-600 dark:text-rose-400"
                      >
                        {{ errors[q.key] }}
                      </p>
                    </fieldset>
                  </div>
                </div>
              </AppCard>
            </div>
          </Transition>
        </div>
      </main>

      <!-- Sticky action bar: reachable with a thumb, clear of the home indicator. -->
      <footer class="glass sticky bottom-0 z-30 border-t border-line">
        <div
          class="mx-auto flex max-w-[720px] items-center gap-3 px-4 pt-3 pb-[max(0.75rem,env(safe-area-inset-bottom))] sm:px-6"
        >
          <AppButton
            v-if="stepIndex > 0"
            variant="secondary"
            size="lg"
            :disabled="submitting"
            @click="goPrev"
          >
            <ArrowLeft class="size-4" />
            上一步
          </AppButton>

          <div class="ml-auto flex items-center gap-3">
            <span class="hidden text-xs text-subtle sm:block">
              {{ stepIndex + 1 }} / {{ steps.length }}
            </span>
            <AppButton size="lg" :loading="submitting" @click="goNext">
              <template v-if="isLast">
                <Send class="size-4" />
                提交
              </template>
              <template v-else>
                下一步
                <ArrowRight class="size-4" />
              </template>
            </AppButton>
          </div>
        </div>
      </footer>
    </template>
  </div>
</template>

<style scoped>
/*
 * `slide-next` / `slide-prev` in main.css pin both panels absolutely, which
 * collapses this wrapper mid-slide and makes the page jump. Stacking them in one
 * grid cell keeps them in flow so the wrapper tracks the taller panel.
 */
.steps-viewport {
  display: grid;
}
.steps-viewport > * {
  grid-area: 1 / 1;
  min-width: 0;
}
.steps-viewport > .slide-next-enter-active,
.steps-viewport > .slide-next-leave-active,
.steps-viewport > .slide-prev-enter-active,
.steps-viewport > .slide-prev-leave-active {
  position: static;
  inset: auto;
}
</style>
