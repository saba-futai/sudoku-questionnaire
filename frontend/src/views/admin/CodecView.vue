<script setup lang="ts">
/**
 * CodecView — the sudoku codec inspector.
 *
 * This page exists to make an invisible process visible. Everything else in the
 * admin shows decoded plaintext; here the admin sees the ciphertext that is
 * actually on disk, watches a byte become a four-clue sudoku, and gets two
 * independent implementations (the browser's and the server's) to agree in
 * front of them.
 *
 * See spec/CODEC.md for the normative description of everything shown here.
 */
import {
  ArrowRight,
  Binary,
  Check,
  Copy,
  Database,
  Eye,
  EyeOff,
  Fingerprint,
  Hash,
  KeyRound,
  Layers,
  RefreshCw,
  ShieldCheck,
  ShieldX,
  Shuffle,
  Sparkles,
  TriangleAlert,
  X,
} from 'lucide-vue-next';
import { computed, onMounted, ref, watch } from 'vue';

import { api } from '@/api/client';
import type { CodecInfo, DecodeReport, QuestionnaireSummary, RawResponse, ResponseView } from '@/api/types';
import AppButton from '@/components/ui/AppButton.vue';
import AppCard from '@/components/ui/AppCard.vue';
import AppSelect from '@/components/ui/AppSelect.vue';
import AppSkeleton from '@/components/ui/AppSkeleton.vue';
import EmptyState from '@/components/ui/EmptyState.vue';
import SudokuMark from '@/components/ui/SudokuMark.vue';
import { utf8Decode, utf8Encode } from '@/codec';
import { grids, gridsSha256 } from '@/codec/grids';
import { parseLayoutId, type LayoutId } from '@/codec/layout';
import { PINNED_GRIDS_SHA256, PINNED_POSSETS_SHA256, PINNED_VALIDITY_SHA256 } from '@/codec/pins';
import { posSetsSha256 } from '@/codec/possets';
import { errorMessage, toast } from '@/lib/toast';
import { useCodecStore } from '@/stores/codec';

/** The demo entry has no server-side twin; it is a table derived from a fixed key. */
const DEMO_KEY = 'qrforqa-demo-table';
const DEMO_OPTION = { value: 'demo', label: '演示表(独立密钥)' };

/** One plaintext byte, its chosen clues, and the grid those clues determine. */
interface ByteInspection {
  byte: number;
  hex: string;
  encodings: number;
  clues: { group: number; pos: number; digit: number; symbol: string }[];
  /** How many of the 288 grids fit the four clues: the spec demands exactly 1. */
  matches: number;
  grid: number[] | null;
}

interface EncodeResult {
  cipher: string;
  length: number;
  plaintextLen: number;
  expansion: number;
  inspection: DecodeReport;
}

const store = useCodecStore();

const questionnaires = ref<QuestionnaireSummary[]>([]);
const listLoading = ref(true);
const listError = ref('');

const selected = ref('');
const isDemo = computed(() => selected.value === DEMO_OPTION.value);
const numericId = computed(() => (isDemo.value ? 0 : Number(selected.value) || 0));

const info = ref<CodecInfo | null>(null);
const infoLoading = ref(false);
const infoError = ref('');

const options = computed(() => [
  DEMO_OPTION,
  ...questionnaires.value.map((q) => ({ value: String(q.id), label: q.title || q.slug })),
]);

/** The key the browser derives its table from: a per-questionnaire secret. */
const tableKey = computed(() =>
  isDemo.value ? DEMO_KEY : (info.value?.derivedKey ?? ''),
);
const layoutId = computed<LayoutId>(() =>
  isDemo.value ? 'ascii' : parseLayoutId(info.value?.layout),
);

// ------------------------------------------------------------- selection ---

async function loadQuestionnaires() {
  listLoading.value = true;
  listError.value = '';
  try {
    const data = await api<{ items: QuestionnaireSummary[] }>('/api/admin/questionnaires');
    questionnaires.value = data.items;
    if (!selected.value) {
      selected.value = data.items.length ? String(data.items[0].id) : DEMO_OPTION.value;
    }
  } catch (err) {
    listError.value = errorMessage(err);
  } finally {
    listLoading.value = false;
  }
}

async function loadInfo() {
  info.value = null;
  infoError.value = '';
  if (isDemo.value || !numericId.value) return;
  infoLoading.value = true;
  try {
    info.value = await api<CodecInfo>(`/api/admin/questionnaires/${numericId.value}/codec`);
  } catch (err) {
    infoError.value = errorMessage(err);
  } finally {
    infoLoading.value = false;
  }
}

watch(selected, async () => {
  // The plaintext is deliberately kept: re-encoding the same text under another
  // questionnaire's key produces a completely different ciphertext, which is
  // the "同一答案在不同问卷中形态不同" property from §12, shown rather than told.
  cipher.value = '';
  previousCipher.value = '';
  previousDecoded.value = '';
  batchCount.value = 0;
  serverEncode.value = null;
  verify.value = { state: 'idle', detail: '' };
  await loadInfo();
  await loadRecent();
});

onMounted(async () => {
  await loadQuestionnaires();
  await Promise.all([loadInfo(), loadRecent()]);
});

// ------------------------------------------------------------ translator ---

const plaintext = ref('每字节一个数独');
const cipher = ref('');
const encoding = ref(false);
const encodeError = ref('');
const decodeError = ref('');
const copiedCipher = ref(false);
const batchCount = ref(0);
const previousCipher = ref('');
const previousDecoded = ref('');
const previousMatches = ref<boolean | null>(null);
const serverEncode = ref<EncodeResult | null>(null);

/**
 * Set while the plaintext is written by the decoder rather than by the admin,
 * so the encode watcher does not immediately overwrite what was pasted.
 */
let fromDecode = false;

/** Entropy-layout ciphertext is binary; escape it so it can be shown at all. */
function escapeWire(value: string): string {
  let out = '';
  for (let i = 0; i < value.length; i += 1) {
    const code = value.charCodeAt(i);
    out += code >= 0x20 && code < 0x7f ? value[i] : `\\x${code.toString(16).padStart(2, '0')}`;
  }
  return out;
}

const cipherDisplay = computed(() =>
  layoutId.value === 'entropy' ? escapeWire(cipher.value) : cipher.value,
);

const plaintextBytes = computed(() => utf8Encode(plaintext.value).length);
const cipherBytes = computed(() => cipher.value.length);
const expansion = computed(() =>
  plaintextBytes.value ? cipherBytes.value / plaintextBytes.value : 0,
);

const verify = ref<{ state: 'idle' | 'busy' | 'ok' | 'fail'; detail: string }>({
  state: 'idle',
  detail: '',
});
let verifySeq = 0;

async function encodeNow(regenerate = false) {
  if (!tableKey.value) return;
  encoding.value = true;
  encodeError.value = '';
  decodeError.value = '';
  try {
    const out = await store.encodeBytes(utf8Encode(plaintext.value), tableKey.value, layoutId.value);
    if (regenerate) {
      previousCipher.value = cipher.value;
      previousDecoded.value = '';
      previousMatches.value = null;
      batchCount.value += 1;
    }
    cipher.value = out;
    void verifyAgainstServer();
  } catch (err) {
    encodeError.value = errorMessage(err);
  } finally {
    encoding.value = false;
  }
}

/**
 * Decodes whatever is in the right-hand panel. The admin can paste a ciphertext
 * copied out of the database and watch it turn back into plaintext.
 */
async function decodeNow() {
  if (!tableKey.value || !cipher.value) return;
  decodeError.value = '';
  try {
    const bytes = await store.decodeBytes(cipher.value, tableKey.value, layoutId.value);
    fromDecode = true;
    plaintext.value = utf8Decode(bytes);
  } catch (err) {
    // A bad frame or a stray byte is a hard error by design (§10), and the
    // message is the explanation.
    decodeError.value = errorMessage(err);
  }
}

let encodeTimer = 0;
watch(plaintext, () => {
  if (fromDecode) {
    fromDecode = false;
    return;
  }
  window.clearTimeout(encodeTimer);
  encodeTimer = window.setTimeout(() => void encodeNow(), 250);
});

let decodeTimer = 0;
function onCipherInput(value: string) {
  cipher.value = value;
  window.clearTimeout(decodeTimer);
  decodeTimer = window.setTimeout(() => void decodeNow(), 250);
}

/**
 * Makes the two implementations prove themselves on the current text:
 * the server's ciphertext must decode locally, and ours must decode on the
 * server. Either one alone would miss a pair that is self-consistent but wrong.
 */
async function verifyAgainstServer() {
  const id = numericId.value;
  if (!id || !plaintext.value || !cipher.value) {
    verify.value = { state: 'idle', detail: isDemo.value ? '演示表没有服务端对照' : '' };
    return;
  }
  const seq = ++verifySeq;
  verify.value = { state: 'busy', detail: '' };
  try {
    const encoded = await api<EncodeResult>('/api/admin/codec/encode', {
      method: 'POST',
      body: { questionnaireId: id, text: plaintext.value, layout: layoutId.value },
    });
    if (seq !== verifySeq) return;
    const decodedLocally = utf8Decode(
      await store.decodeBytes(encoded.cipher, tableKey.value, layoutId.value),
    );
    const decodedRemotely = await api<{ text: string }>('/api/admin/codec/decode', {
      method: 'POST',
      body: { questionnaireId: id, cipher: cipher.value, layout: layoutId.value },
    });
    if (seq !== verifySeq) return;
    serverEncode.value = encoded;
    const ok = decodedLocally === plaintext.value && decodedRemotely.text === plaintext.value;
    verify.value = ok
      ? { state: 'ok', detail: '服务端密文本地解得开,本地密文服务端也解得开' }
      : {
          state: 'fail',
          detail: `互解结果不一致:本地解服务端 ${decodedLocally === plaintext.value ? '一致' : '不一致'},服务端解本地 ${decodedRemotely.text === plaintext.value ? '一致' : '不一致'}`,
        };
  } catch (err) {
    if (seq !== verifySeq) return;
    verify.value = { state: 'fail', detail: errorMessage(err) };
    serverEncode.value = null;
  }
}

async function provePrevious() {
  if (!previousCipher.value) return;
  try {
    previousDecoded.value = utf8Decode(
      await store.decodeBytes(previousCipher.value, tableKey.value, layoutId.value),
    );
    previousMatches.value = previousDecoded.value === plaintext.value;
  } catch (err) {
    previousDecoded.value = errorMessage(err);
    previousMatches.value = false;
  }
}

async function copyCipher() {
  try {
    await navigator.clipboard.writeText(cipher.value);
    copiedCipher.value = true;
    toast.success('密文已复制');
    window.setTimeout(() => (copiedCipher.value = false), 2000);
  } catch {
    toast.error('复制失败', '请手动选中密文复制');
  }
}

async function copy(value: string, label: string) {
  try {
    await navigator.clipboard.writeText(value);
    toast.success(`${label}已复制`);
  } catch {
    toast.error('复制失败', '请手动选中复制');
  }
}

// ----------------------------------------------------------- local table ---

const codecReady = computed(() => store.status === 'ready' && !!store.codec && !!tableKey.value);

/** The browser's own derived table for the selected questionnaire. */
const localStats = computed(() => {
  const instance = store.codec;
  if (store.status !== 'ready' || !instance || !tableKey.value) return null;
  const table = instance.table(tableKey.value, layoutId.value);
  let min = Number.MAX_SAFE_INTEGER;
  let max = 0;
  for (let b = 0; b < 256; b += 1) {
    const n = table.encodingCount(b);
    if (n < min) min = n;
    if (n > max) max = n;
  }
  return { min, max, distinct: table.decodeMapSize(), fingerprint: table.fingerprint };
});

/** Local digests, for the pinned-value comparison. */
const localDigests = computed(() => {
  void store.status;
  return { grids: gridsSha256(), possets: posSetsSha256() };
});

const integrity = computed(() => {
  const server = info.value?.fingerprint ?? '';
  const local = localStats.value?.fingerprint ?? '';
  return {
    server,
    local,
    available: !!server && !!local,
    match: !!server && !!local && server === local,
  };
});

const gridsPinned = computed(() => localDigests.value.grids === PINNED_GRIDS_SHA256);
const posSetsPinned = computed(() => localDigests.value.possets === PINNED_POSSETS_SHA256);

// -------------------------------------------------------- byte inspection ---

/**
 * Rebuilds, in the browser, the puzzle the encoder actually emitted for one
 * plaintext byte: take the four clue symbols that are really in the ciphertext,
 * turn each 6-bit group back into a cell and a digit, then search the 288
 * enumerated grids for the one that fits.
 *
 * Finding exactly one is the property the whole scheme rests on — the search is
 * the proof, and it is cheap enough to run on demand.
 */
function inspectCipherByte(index: number): ByteInspection | null {
  const instance = store.codec;
  if (!instance || !tableKey.value) return null;
  const table = instance.table(tableKey.value, layoutId.value);

  // Four wire symbols per plaintext byte, in the order the encoder happened to
  // permute them — the decode map is order-independent (see spec §7.1).
  const symbols: number[] = [];
  for (let i = index * 4; i < index * 4 + 4; i += 1) {
    if (i >= cipher.value.length) return null;
    symbols.push(cipher.value.charCodeAt(i) & 0xff);
  }
  const groups = symbols.map((b) => table.layout.groupOf(b));
  if (groups.some((g) => g < 0)) return null;

  const byte = table.lookup(groups[0], groups[1], groups[2], groups[3]);
  if (byte < 0) return null;

  const clues = groups.map((g, i) => ({
    group: g,
    pos: g & 0x0f,
    digit: (g >> 4) + 1,
    symbol: cipher.value[index * 4 + i] as string,
  }));
  const fitting = grids().filter((g) => clues.every((c) => g[c.pos] === c.digit));
  return {
    byte,
    hex: byte.toString(16).padStart(2, '0').toUpperCase(),
    encodings: table.encodingCount(byte),
    clues,
    matches: fitting.length,
    grid: fitting.length === 1 ? Array.from(fitting[0]) : null,
  };
}

/** The first few bytes of the current ciphertext, visualised. */
const inspections = computed<ByteInspection[]>(() => {
  if (!codecReady.value) return [];
  const out: ByteInspection[] = [];
  for (let i = 0; i < 4; i += 1) {
    const item = inspectCipherByte(i);
    if (item) out.push(item);
  }
  return out;
});

/** The plaintext byte a card is about, for the caption. */
function plaintextByte(index: number): number | null {
  const bytes = utf8Encode(plaintext.value);
  return index < bytes.length ? (bytes[index] as number) : null;
}

/** A 4x4 with only the four clue cells filled — the cells the encoder chose. */
function clueGrid(inspection: ByteInspection): number[] {
  const out = new Array<number>(16).fill(0);
  for (const c of inspection.clues) out[c.pos] = c.digit;
  return out;
}

// ------------------------------------------------------------- on-disk ---

const recent = ref<ResponseView[]>([]);
const recentLoading = ref(false);
const recentError = ref('');
const selectedResponseId = ref<number | null>(null);
const record = ref<RawResponse | null>(null);
const recordLoading = ref(false);
const recordError = ref('');
const copiedRecord = ref(false);

const selectedResponse = computed(
  () => recent.value.find((r) => r.id === selectedResponseId.value) ?? null,
);

/** The decoded answers of the same record, so the two can be read side by side. */
const decodedRecord = computed(() => {
  const row = selectedResponse.value;
  if (!row) return '';
  return JSON.stringify(row.answers, null, 2);
});

async function loadRecent() {
  recent.value = [];
  selectedResponseId.value = null;
  record.value = null;
  recordError.value = '';
  if (isDemo.value || !numericId.value) return;
  recentLoading.value = true;
  recentError.value = '';
  try {
    const data = await api<{ items: ResponseView[] }>(
      `/api/admin/questionnaires/${numericId.value}/responses?size=5&order=desc`,
    );
    recent.value = data.items;
    if (data.items.length) await selectResponse(data.items[0].id);
  } catch (err) {
    recentError.value = errorMessage(err);
  } finally {
    recentLoading.value = false;
  }
}

async function selectResponse(id: number) {
  selectedResponseId.value = id;
  record.value = null;
  recordError.value = '';
  recordLoading.value = true;
  try {
    record.value = await api<RawResponse>(
      `/api/admin/questionnaires/${numericId.value}/responses/${id}/raw`,
    );
  } catch (err) {
    recordError.value = errorMessage(err);
  } finally {
    recordLoading.value = false;
  }
}

async function copyRecordCipher() {
  if (!record.value) return;
  try {
    await navigator.clipboard.writeText(record.value.cipher);
    copiedRecord.value = true;
    toast.success('密文已复制');
    window.setTimeout(() => (copiedRecord.value = false), 2000);
  } catch {
    toast.error('复制失败', '请手动选中密文复制');
  }
}

// ---------------------------------------------------------------- misc ---

const showKey = ref(false);
const maskedKey = computed(() => {
  const key = tableKey.value;
  if (!key) return '—';
  return `${'•'.repeat(Math.max(0, key.length - 6))}${key.slice(-6)}`;
});
const layoutLabel = computed(() => (layoutId.value === 'entropy' ? '熵档(二进制)' : 'ASCII 档'));

const PIPELINE = [
  { title: '明文', detail: '题目与答案先序列化成 UTF-8 字节,一个字节一个字节地处理。' },
  { title: '每字节一个唯一解数独', detail: '每个字节在密钥表里对应一批线索组合,每个组合恰好只被一个 4×4 数独满足。' },
  { title: '拼接为密文', detail: '四格线索 → 四个符号,再加四个符号的 CRC 尾帧,密文长度恒为明文的 4 倍多 4。' },
  { title: '落库', detail: 'SQLite 里只有这些符号。解码要密钥表,而表只存在于服务端与这份前端包里。' },
];

onMounted(() => {
  if (plaintext.value) void encodeNow(true);
});

watch([tableKey, layoutId], () => {
  if (plaintext.value) void encodeNow(true);
});
</script>

<template>
  <div class="mx-auto w-full max-w-7xl">
    <!-- ------------------------------------------------------- header --- -->
    <!-- The layout already prints the route title, so this is the subtitle. -->
    <header class="mb-6 flex items-center gap-3">
      <span
        class="grid size-9 shrink-0 place-items-center rounded-xl bg-brand-50 text-brand-600 dark:bg-brand-500/15 dark:text-brand-300"
      >
        <Sparkles class="size-5" />
      </span>
      <p class="text-sm text-muted">
        数据库里只有数独符号。这里把「明文变成密文」的每一步摊开来看。
      </p>
    </header>

    <!-- ----------------------------------------------------- selector --- -->
    <AppCard padding="sm" class="mb-5">
      <div class="flex flex-wrap items-end gap-3">
        <div class="min-w-[220px] flex-1">
          <AppSelect v-model="selected" label="检视对象" :options="options" />
        </div>
        <div class="flex flex-wrap items-center gap-2 pb-1 text-xs text-subtle">
          <span class="rounded-full bg-sunken px-2 py-1 font-mono">{{ layoutLabel }}</span>
          <span v-if="store.status === 'loading'" class="tabular-nums">正在加载校验位图…</span>
          <span v-else-if="store.status === 'error'" class="text-rose-600 dark:text-rose-400">
            {{ store.error }}
          </span>
          <span v-else-if="store.status === 'ready'" class="tabular-nums">浏览器编解码器已就绪</span>
        </div>
      </div>
      <p v-if="listError" class="mt-2 flex items-center gap-2 text-xs text-rose-600 dark:text-rose-400">
        <TriangleAlert class="size-3.5" />
        {{ listError }}
        <AppButton variant="ghost" size="sm" @click="loadQuestionnaires">重试</AppButton>
      </p>
    </AppCard>

    <div v-if="listLoading" class="space-y-4">
      <AppSkeleton variant="card" :lines="4" />
      <AppSkeleton variant="card" :lines="4" />
    </div>

    <AppCard v-else-if="infoError" padding="lg" class="mb-5">
      <EmptyState
        :icon="TriangleAlert"
        title="无法读取该问卷的编码表信息"
        :description="infoError"
      >
        <AppButton variant="secondary" @click="loadInfo">重试</AppButton>
      </EmptyState>
    </AppCard>

    <template v-else>
      <!-- ================================================ translator === -->
      <section class="mb-5">
        <div class="mb-3 flex flex-wrap items-center justify-between gap-2">
          <h2 class="text-base font-semibold text-ink">实时对照</h2>
          <div class="flex flex-wrap items-center gap-2 text-xs">
            <span class="rounded-full bg-sunken px-2 py-1 text-muted tabular-nums">
              明文 {{ plaintextBytes }} B → 密文 {{ cipherBytes }} B
            </span>
            <span
              class="rounded-full px-2 py-1 font-medium tabular-nums"
              :class="
                expansion > 0
                  ? 'bg-brand-50 text-brand-700 dark:bg-brand-500/15 dark:text-brand-300'
                  : 'bg-sunken text-muted'
              "
            >
              膨胀 {{ expansion ? expansion.toFixed(2) : '—' }}×
            </span>
          </div>
        </div>

        <div class="grid gap-4 lg:grid-cols-2">
          <!-- plaintext -->
          <AppCard padding="none" class="overflow-hidden">
            <div class="flex items-center justify-between border-b border-line px-4 py-2.5">
              <span class="text-sm font-medium text-ink">明文</span>
              <span class="font-mono text-[11px] text-subtle">一个字节 = 一个数独</span>
            </div>
            <textarea
              v-model="plaintext"
              rows="7"
              spellcheck="false"
              class="cipher block w-full resize-y bg-transparent px-4 py-3 text-ink outline-none placeholder:text-subtle"
              placeholder="在这里输入,右侧实时变成数独密文"
            />
            <div class="flex items-center justify-between border-t border-line px-4 py-2">
              <span class="text-xs text-subtle tabular-nums">{{ plaintext.length }} 个字符</span>
              <span v-if="encoding" class="text-xs text-subtle">编码中…</span>
            </div>
          </AppCard>

          <!-- ciphertext -->
          <AppCard padding="none" class="overflow-hidden">
            <div class="flex items-center justify-between border-b border-line px-4 py-2.5">
              <span class="text-sm font-medium text-ink">密文</span>
              <div class="flex items-center gap-1">
                <AppButton variant="ghost" size="sm" @click="copyCipher">
                  <component :is="copiedCipher ? Check : Copy" class="size-3.5" />
                  复制
                </AppButton>
                <AppButton variant="ghost" size="sm" :loading="encoding" @click="encodeNow(true)">
                  <Shuffle class="size-3.5" />
                  换一批编码
                </AppButton>
              </div>
            </div>
            <textarea
              :value="cipherDisplay"
              rows="7"
              spellcheck="false"
              :readonly="layoutId === 'entropy'"
              class="cipher block w-full resize-y bg-transparent px-4 py-3 text-ink outline-none"
              :placeholder="layoutId === 'entropy' ? '熵档密文为二进制,仅以转义形式展示' : '粘贴密文,左侧实时解回明文'"
              @input="onCipherInput(($event.target as HTMLTextAreaElement).value)"
            />
            <div class="flex items-center justify-between border-t border-line px-4 py-2">
              <span class="text-xs text-subtle tabular-nums">
                {{ cipherBytes }} 字节 · 第 {{ batchCount + 1 }} 批
              </span>
              <span
                v-if="verify.state !== 'idle'"
                class="flex items-center gap-1 text-xs"
                :class="
                  verify.state === 'ok'
                    ? 'text-emerald-600 dark:text-emerald-400'
                    : verify.state === 'fail'
                      ? 'text-rose-600 dark:text-rose-400'
                      : 'text-subtle'
                "
              >
                <component
                  :is="verify.state === 'ok' ? ShieldCheck : verify.state === 'fail' ? ShieldX : RefreshCw"
                  class="size-3.5"
                  :class="verify.state === 'busy' ? 'animate-spin' : ''"
                />
                {{ verify.state === 'ok' ? '与服务器互解一致' : verify.state === 'fail' ? '互解不一致' : '校验中…' }}
              </span>
            </div>
          </AppCard>
        </div>

        <!-- the surprising property, stated plainly -->
        <AppCard v-if="previousCipher" padding="sm" class="mt-3 border-brand-200 bg-brand-50/50 dark:border-brand-500/25 dark:bg-brand-500/5">
          <div class="flex flex-wrap items-start gap-3">
            <Shuffle class="mt-0.5 size-4 shrink-0 text-brand-600 dark:text-brand-300" />
            <div class="min-w-0 flex-1">
              <p class="text-sm text-ink">
                <span class="font-medium">密文变了,明文没变。</span>
                编码时每个字节都会随机换一个数独、再随机打乱四格的顺序,所以同一段明文每次编码都得到不同的密文,
                却都能解回同一段明文。这正是「同一句话在库里长得不一样」的原因。
              </p>
              <div class="cipher mt-2 max-h-20 overflow-y-auto rounded-lg border border-line bg-sunken p-2 text-subtle">
                {{ layoutId === 'entropy' ? escapeWire(previousCipher) : previousCipher }}
              </div>
              <div class="mt-2 flex flex-wrap items-center gap-2">
                <AppButton variant="secondary" size="sm" @click="provePrevious">解出这段旧密文</AppButton>
                <span
                  v-if="previousMatches !== null"
                  class="flex items-center gap-1 text-xs"
                  :class="
                    previousMatches
                      ? 'text-emerald-600 dark:text-emerald-400'
                      : 'text-rose-600 dark:text-rose-400'
                  "
                >
                  <component :is="previousMatches ? Check : X" class="size-3.5" />
                  {{ previousMatches ? '解出的明文与左侧完全一致' : '解出的明文不一致' }}
                </span>
              </div>
              <p v-if="previousDecoded && previousMatches !== null" class="cipher mt-2 text-xs text-muted">
                {{ previousDecoded }}
              </p>
            </div>
          </div>
        </AppCard>

        <p v-if="encodeError" class="mt-2 text-xs text-rose-600 dark:text-rose-400">{{ encodeError }}</p>
        <p v-if="decodeError" class="mt-2 flex items-start gap-1.5 text-xs text-rose-600 dark:text-rose-400">
          <TriangleAlert class="mt-0.5 size-3.5 shrink-0" />
          {{ decodeError }}
        </p>
        <p v-else-if="verify.state === 'fail' && verify.detail" class="mt-2 text-xs text-rose-600 dark:text-rose-400">
          {{ verify.detail }}
        </p>
      </section>

      <!-- ============================================ grid visualiser === -->
      <section class="mb-5">
        <div class="mb-3">
          <h2 class="text-base font-semibold text-ink">四个线索,唯一确定一个数独</h2>
          <p class="mt-1 text-sm text-muted">
            取上面那段密文的前几个字节:每个明文字节对应四个线索格,而全部 288 个 4×4 数独中,
            只有一个与这四个格子完全吻合 —— 这就是它为什么能还原成恰好一个字节。
            换一批编码后这里会跟着变,因为每个字节都换了一个数独。
          </p>
        </div>

        <AppCard v-if="!codecReady" padding="lg">
          <EmptyState
            :icon="Database"
            title="编解码器尚未就绪"
            :description="
              store.status === 'error'
                ? store.error
                : '正在加载并校验数独校验位图,完成后这里会显示逐字节的还原过程。'
            "
          >
            <AppButton v-if="store.status === 'error'" variant="secondary" @click="store.load()">
              重试
            </AppButton>
          </EmptyState>
        </AppCard>

        <AppCard v-else-if="inspections.length === 0" padding="lg">
          <EmptyState
            title="没有可展示的字节"
            description="在上面的明文框里输入内容,密文出现了这里就会跟着出现。"
          />
        </AppCard>

        <div v-else class="stagger grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
          <AppCard
            v-for="(item, i) in inspections"
            :key="`${batchCount}-${i}-${item.byte}`"
            padding="sm"
            :style="{ '--i': i }"
          >
            <div class="flex items-start justify-between gap-2">
              <div>
                <p class="text-xs text-subtle">明文的第 {{ i + 1 }} 个字节</p>
                <p class="mt-0.5 font-mono text-sm text-ink">
                  0x{{ item.hex }}
                  <span v-if="plaintextByte(i) !== null" class="text-subtle">
                    · {{ item.byte }}
                  </span>
                </p>
              </div>
              <span
                class="rounded-full px-2 py-0.5 text-[11px] font-medium tabular-nums"
                :class="
                  item.matches === 1
                    ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-300'
                    : 'bg-rose-50 text-rose-700 dark:bg-rose-500/10 dark:text-rose-300'
                "
              >
                {{ item.matches === 1 ? '唯一解' : `${item.matches} 个匹配` }}
              </span>
            </div>

            <div class="mt-3 flex flex-wrap items-center justify-center gap-3">
              <!-- the four cells the encoder picked -->
              <div>
                <div class="sudoku-grid" style="grid-template-columns: repeat(4, min-content)">
                  <div
                    v-for="(cell, ci) in clueGrid(item)"
                    :key="ci"
                    class="sudoku-cell size-5 text-[10px]"
                    :data-box-right="ci % 4 === 1 ? '1' : undefined"
                    :data-box-bottom="Math.floor(ci / 4) === 1 ? '1' : undefined"
                    :class="cell ? 'animate-cell-pop text-brand-600 dark:text-brand-300' : 'opacity-40'"
                  >
                    {{ cell || '·' }}
                  </div>
                </div>
                <p class="mt-1 text-center text-[10px] text-subtle">四个线索格</p>
              </div>

              <ArrowRight class="size-4 shrink-0 text-subtle" />

              <!-- the grid those four cells force -->
              <div>
                <SudokuMark v-if="item.grid" :grid="item.grid" size="sm" :animate="true" />
                <div v-else class="grid size-20 place-items-center text-xs text-rose-500">无解</div>
                <p class="mt-1 text-center text-[10px] text-subtle">唯一确定的数独</p>
              </div>
            </div>

            <div class="mt-3 flex flex-wrap gap-1.5">
              <span
                v-for="c in item.clues"
                :key="c.pos"
                class="rounded-md bg-sunken px-1.5 py-0.5 font-mono text-[11px] text-muted tabular-nums"
                :title="`第 ${c.pos} 格 = ${c.digit}`"
              >
                <span class="text-ink">{{ c.symbol }}</span>
                {{ c.pos }}↦{{ c.digit }}
              </span>
            </div>

            <p class="mt-2 text-[11px] text-subtle tabular-nums">
              四格排序后查表 → 字节 0x{{ item.hex }} · 共 {{ item.encodings }} 种合法编码
            </p>
          </AppCard>
        </div>
      </section>

      <!-- ========================================== table properties === -->
      <section class="mb-5 grid items-start gap-4 lg:grid-cols-2">
        <AppCard>
          <template #title>
            <h2 class="text-base font-semibold text-ink">编码表属性</h2>
            <p class="mt-0.5 text-xs text-subtle">
              这张表由问卷 ID 派生而来,是本问卷独有的
            </p>
          </template>

          <div v-if="infoLoading" class="space-y-2">
            <AppSkeleton variant="text" :lines="5" />
          </div>

          <dl v-else class="space-y-3 text-sm">
            <div class="flex items-start justify-between gap-4">
              <dt class="flex items-center gap-1.5 text-muted">
                <KeyRound class="size-3.5" />
                派生密钥
              </dt>
              <dd class="flex min-w-0 items-center gap-1">
                <code class="cipher truncate text-ink">{{ showKey ? tableKey || '—' : maskedKey }}</code>
                <AppButton variant="ghost" size="sm" :title="showKey ? '隐藏' : '显示'" @click="showKey = !showKey">
                  <component :is="showKey ? EyeOff : Eye" class="size-3.5" />
                </AppButton>
                <AppButton variant="ghost" size="sm" title="复制" @click="copy(tableKey, '密钥')">
                  <Copy class="size-3.5" />
                </AppButton>
              </dd>
            </div>

            <div class="flex items-start justify-between gap-4">
              <dt class="flex items-center gap-1.5 text-muted">
                <Fingerprint class="size-3.5" />
                指纹
              </dt>
              <dd class="flex min-w-0 items-center gap-1">
                <code class="cipher truncate text-ink">{{ integrity.server || localStats?.fingerprint || '—' }}</code>
                <AppButton
                  variant="ghost"
                  size="sm"
                  title="复制指纹"
                  @click="copy(integrity.server || localStats?.fingerprint || '', '指纹')"
                >
                  <Copy class="size-3.5" />
                </AppButton>
              </dd>
            </div>

            <div class="grid grid-cols-2 gap-3 pt-1 sm:grid-cols-4">
              <div class="rounded-[var(--radius-control)] border border-line p-2.5">
                <dt class="text-xs text-subtle">网格数</dt>
                <dd class="mt-0.5 font-mono text-ink tabular-nums">
                  {{ info?.gridCount ?? 288 }}
                </dd>
              </div>
              <div class="rounded-[var(--radius-control)] border border-line p-2.5">
                <dt class="text-xs text-subtle">位置组合</dt>
                <dd class="mt-0.5 font-mono text-ink tabular-nums">
                  {{ info?.posSetCount ?? 1820 }}
                </dd>
              </div>
              <div class="rounded-[var(--radius-control)] border border-line p-2.5">
                <dt class="text-xs text-subtle">每字节编码数</dt>
                <dd class="mt-0.5 font-mono text-ink tabular-nums">
                  {{ localStats ? `${localStats.min}–${localStats.max}` : '—' }}
                </dd>
              </div>
              <div class="rounded-[var(--radius-control)] border border-line p-2.5">
                <dt class="text-xs text-subtle">不同编码</dt>
                <dd class="mt-0.5 font-mono text-ink tabular-nums">
                  {{ localStats?.distinct ?? info?.distinctPuzzles ?? '—' }}
                </dd>
              </div>
            </div>

            <p class="text-xs text-subtle">
              每字节的合法编码数只有 12 与 128 两种取值 —— 这来自 4×4 数独的两个对称轨道,
              是枚举正确性的一个旁证(见规范 §4)。
            </p>
          </dl>
        </AppCard>

        <AppCard>
          <template #title>
            <h2 class="text-base font-semibold text-ink">一致性校验</h2>
            <p class="mt-0.5 text-xs text-subtle">浏览器与服务端各自算一遍,结果必须一致</p>
          </template>

          <div class="space-y-3">
            <!-- server vs browser -->
            <div
              class="flex items-start gap-3 rounded-[var(--radius-control)] border p-3"
              :class="
                integrity.available && integrity.match
                  ? 'border-emerald-200 bg-emerald-50/60 dark:border-emerald-500/25 dark:bg-emerald-500/5'
                  : 'border-line'
              "
            >
              <component
                :is="integrity.available && integrity.match ? ShieldCheck : ShieldX"
                class="mt-0.5 size-4 shrink-0"
                :class="
                  integrity.available && integrity.match
                    ? 'text-emerald-600 dark:text-emerald-400'
                    : 'text-subtle'
                "
              />
              <div class="min-w-0 text-sm">
                <p class="font-medium text-ink">
                  {{
                    isDemo
                      ? '演示表没有服务端对照'
                      : !integrity.available
                        ? '等待本地表或服务端指纹'
                        : integrity.match
                          ? '本地指纹 = 服务端指纹'
                          : '本地指纹 ≠ 服务端指纹'
                  }}
                </p>
                <p v-if="integrity.available" class="mt-1 font-mono text-[11px] break-all text-subtle">
                  本地 {{ integrity.local.slice(0, 24) }}…<br />
                  服务端 {{ integrity.server.slice(0, 24) }}…
                </p>
                <p v-else class="mt-1 text-xs text-subtle">
                  指纹刻画的是密钥派生结果,与布局无关。
                </p>
              </div>
            </div>

            <!-- pinned digests -->
            <div class="space-y-2 rounded-[var(--radius-control)] border border-line p-3">
              <div class="flex items-center justify-between gap-3 text-sm">
                <span class="flex items-center gap-1.5 text-muted">
                  <Hash class="size-3.5" />
                  288 网格 sha256
                </span>
                <span
                  class="flex items-center gap-1 text-xs"
                  :class="gridsPinned ? 'text-emerald-600 dark:text-emerald-400' : 'text-rose-600 dark:text-rose-400'"
                >
                  <component :is="gridsPinned ? Check : X" class="size-3.5" />
                  {{ gridsPinned ? '与固定值一致' : '与固定值不符' }}
                </span>
              </div>
              <code class="cipher block text-[11px] break-all text-subtle">{{ PINNED_GRIDS_SHA256 }}</code>

              <div class="flex items-center justify-between gap-3 pt-1 text-sm">
                <span class="flex items-center gap-1.5 text-muted">
                  <Hash class="size-3.5" />
                  1820 位置组合 sha256
                </span>
                <span
                  class="flex items-center gap-1 text-xs"
                  :class="posSetsPinned ? 'text-emerald-600 dark:text-emerald-400' : 'text-rose-600 dark:text-rose-400'"
                >
                  <component :is="posSetsPinned ? Check : X" class="size-3.5" />
                  {{ posSetsPinned ? '与固定值一致' : '与固定值不符' }}
                </span>
              </div>
              <code class="cipher block text-[11px] break-all text-subtle">{{ PINNED_POSSETS_SHA256 }}</code>

              <div class="flex items-center justify-between gap-3 pt-1 text-sm">
                <span class="flex items-center gap-1.5 text-muted">
                  <Hash class="size-3.5" />
                  校验位图 sha256
                </span>
                <span class="text-xs text-subtle">下载时已在浏览器中校验</span>
              </div>
              <code class="cipher block text-[11px] break-all text-subtle">{{ PINNED_VALIDITY_SHA256 }}</code>
            </div>

            <p v-if="info?.gridsSha256" class="text-xs text-subtle">
              服务端声明的网格哈希:
              <code class="cipher break-all">{{ info.gridsSha256 }}</code>
            </p>
          </div>
        </AppCard>
      </section>

      <!-- ============================================== on-disk view === -->
      <section class="mb-5">
        <div class="mb-3">
          <h2 class="text-base font-semibold text-ink">库里到底存了什么</h2>
          <p class="mt-1 text-sm text-muted">
            左边是从 SQLite 里原样取出的密文,右边是同一份记录解码后的明文。两者是同一条数据。
          </p>
        </div>

        <AppCard v-if="isDemo" padding="lg">
          <EmptyState
            :icon="Database"
            title="演示表没有真实记录"
            description="选择一份真实问卷,就能看到它在数据库里的样子。"
          />
        </AppCard>

        <AppCard v-else-if="recentLoading" padding="lg">
          <AppSkeleton variant="text" :lines="4" />
        </AppCard>

        <AppCard v-else-if="recentError" padding="lg">
          <EmptyState :icon="TriangleAlert" title="无法读取最近答卷" :description="recentError">
            <AppButton variant="secondary" @click="loadRecent">重试</AppButton>
          </EmptyState>
        </AppCard>

        <AppCard v-else-if="recent.length === 0" padding="lg">
          <EmptyState
            :icon="Database"
            title="这份问卷还没有答卷"
            description="发布问卷并收到提交后,这里会显示真实存储在库中的密文。"
          />
        </AppCard>

        <div v-else class="grid gap-4 lg:grid-cols-2">
          <AppCard padding="none" class="overflow-hidden">
            <div class="flex flex-wrap items-center gap-2 border-b border-line px-4 py-2.5">
              <Binary class="size-4 text-subtle" />
              <span class="text-sm font-medium text-ink">数据库中的密文</span>
              <AppButton
                v-if="record"
                variant="ghost"
                size="sm"
                class="ml-auto"
                @click="copyRecordCipher"
              >
                <component :is="copiedRecord ? Check : Copy" class="size-3.5" />
                {{ copiedRecord ? '已复制' : '复制' }}
              </AppButton>
            </div>

            <div class="flex flex-wrap gap-1.5 border-b border-line px-4 py-2">
              <button
                v-for="row in recent"
                :key="row.id"
                type="button"
                class="rounded-full px-2.5 py-1 text-xs font-medium transition-colors tabular-nums"
                :class="
                  selectedResponseId === row.id
                    ? 'bg-brand-600 text-white dark:bg-brand-500'
                    : 'bg-sunken text-muted hover:text-ink'
                "
                @click="selectResponse(row.id)"
              >
                #{{ row.index }}
              </button>
            </div>

            <div v-if="recordLoading" class="p-4">
              <AppSkeleton variant="text" :lines="4" />
            </div>
            <div v-else-if="recordError" class="p-4">
              <p class="flex items-start gap-1.5 text-xs text-rose-600 dark:text-rose-400">
                <TriangleAlert class="mt-0.5 size-3.5 shrink-0" />
                {{ recordError }}
              </p>
              <AppButton variant="secondary" size="sm" class="mt-2" @click="selectResponse(selectedResponseId ?? 0)">
                重试
              </AppButton>
            </div>
            <div v-else-if="record" class="p-4">
              <div class="cipher max-h-60 overflow-y-auto rounded-lg border border-line bg-sunken p-3 text-muted">
                {{ record.cipher }}
              </div>
              <dl class="mt-3 grid grid-cols-2 gap-2 text-xs">
                <div class="rounded-lg border border-line p-2">
                  <dt class="text-subtle">字节数</dt>
                  <dd class="mt-0.5 font-mono text-ink tabular-nums">{{ record.length }}</dd>
                </div>
                <div class="rounded-lg border border-line p-2">
                  <dt class="text-subtle">线索符号</dt>
                  <dd class="mt-0.5 font-mono text-ink tabular-nums">
                    {{ record.inspection.clueSymbols }}
                  </dd>
                </div>
                <div class="rounded-lg border border-line p-2">
                  <dt class="text-subtle">填充字节</dt>
                  <dd class="mt-0.5 font-mono text-ink tabular-nums">
                    {{ record.inspection.paddingBytes }}
                  </dd>
                </div>
                <div class="rounded-lg border border-line p-2">
                  <dt class="text-subtle">校验和</dt>
                  <dd
                    class="mt-0.5 font-mono"
                    :class="
                      record.inspection.checksumOk
                        ? 'text-emerald-600 dark:text-emerald-400'
                        : 'text-rose-600 dark:text-rose-400'
                    "
                  >
                    {{ record.inspection.checksumOk ? '通过' : '不匹配' }}
                  </dd>
                </div>
                <div class="rounded-lg border border-line p-2">
                  <dt class="text-subtle">明文长度</dt>
                  <dd class="mt-0.5 font-mono text-ink tabular-nums">
                    {{ record.inspection.plaintextLen }}
                  </dd>
                </div>
                <div class="rounded-lg border border-line p-2">
                  <dt class="text-subtle">尾帧</dt>
                  <dd class="mt-0.5 font-mono text-[11px] break-all text-ink">
                    {{ record.inspection.trailerHex || '—' }}
                  </dd>
                </div>
              </dl>
            </div>
          </AppCard>

          <AppCard padding="none" class="overflow-hidden">
            <div class="flex items-center gap-2 border-b border-line px-4 py-2.5">
              <Layers class="size-4 text-subtle" />
              <span class="text-sm font-medium text-ink">解码后的明文</span>
              <span v-if="selectedResponse" class="ml-auto text-xs text-subtle tabular-nums">
                #{{ selectedResponse.index }} · {{ selectedResponse.durationMs }} ms
              </span>
            </div>
            <div class="p-4">
              <pre
                v-if="decodedRecord"
                class="cipher max-h-[22rem] overflow-auto rounded-lg border border-line bg-sunken p-3 text-muted"
              >{{ decodedRecord }}</pre>
              <p v-else class="text-sm text-subtle">选择一条记录。</p>
              <p class="mt-3 text-xs text-subtle">
                这两个面板显示的是同一条记录:上面的密文原样存在数据库里,下面的明文只在此刻、在管理员的浏览器中生成。
              </p>
            </div>
          </AppCard>
        </div>
      </section>

      <!-- ================================================ pipeline === -->
      <section>
        <h2 class="mb-3 text-base font-semibold text-ink">整个流程</h2>
        <div class="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
          <AppCard v-for="(step, i) in PIPELINE" :key="step.title" padding="sm">
            <div class="flex items-center gap-2">
              <span
                class="grid size-6 place-items-center rounded-md bg-brand-50 text-xs font-semibold text-brand-700 tabular-nums dark:bg-brand-500/15 dark:text-brand-300"
              >
                {{ i + 1 }}
              </span>
              <p class="text-sm font-medium text-ink">{{ step.title }}</p>
            </div>
            <p class="mt-2 text-xs text-muted">{{ step.detail }}</p>
          </AppCard>
        </div>
      </section>
    </template>
  </div>
</template>
