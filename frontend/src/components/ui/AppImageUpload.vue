<script setup lang="ts">
/**
 * Image question input.
 *
 * Everything happens in the browser before the bytes ever leave it: the file is
 * decoded, downscaled to a sane dimension, re-encoded, and only then encrypted
 * into ciphertext by the caller's codec. That ordering is the point of the
 * product — the server stores a sudoku, not a photo.
 *
 * Files are processed one at a time on purpose. A phone handed six camera
 * photos at once will happily decode them in parallel until the tab is killed;
 * a sequential loop keeps peak memory to a single canvas.
 */
import { AlertTriangle, Image as ImageIcon, Loader2, Plus, X } from 'lucide-vue-next';
import { computed, onBeforeUnmount, ref } from 'vue';

import { errorMessage } from '@/lib/toast';

export interface ImageUploadPayload {
  cipher: string;
  layout: string;
  mime: string;
  width: number;
  height: number;
  fieldKey: string;
}

type Stage = 'compress' | 'encode' | 'upload';

interface Job {
  key: number;
  name: string;
  stage: Stage;
}

const props = withDefaults(
  defineProps<{
    modelValue?: number[];
    maxImages?: number;
    maxDimension?: number;
    quality?: number;
    accept?: string;
    disabled?: boolean;
    uploadFn: (payload: ImageUploadPayload) => Promise<{ id: number }>;
    fieldKey: string;
    layout: string;
    codecEncode: (base64: string) => Promise<string>;
  }>(),
  {
    modelValue: () => [],
    maxImages: 1,
    maxDimension: 1600,
    quality: 0.82,
    accept: 'image/*',
  },
);

const emit = defineEmits<{
  'update:modelValue': [number[]];
  error: [string];
}>();

const fileInput = ref<HTMLInputElement | null>(null);
/** Object URLs for images uploaded in this session; ids from elsewhere have none. */
const previews = ref<Record<number, string>>({});
const jobs = ref<Job[]>([]);
const failure = ref('');

let seq = 0;
/** Sources below this stay PNG so a small logo keeps its transparency. */
const PNG_KEEP_LIMIT = 300 * 1024;

const ids = computed(() => props.modelValue ?? []);
const full = computed(() => ids.value.length + jobs.value.length >= props.maxImages);

const STAGE_LABEL: Record<Stage, string> = {
  compress: '压缩中',
  encode: '加密中',
  upload: '上传中',
};

function report(message: string) {
  failure.value = message;
  emit('error', message);
}

function openPicker() {
  if (props.disabled) return;
  fileInput.value?.click();
}

async function onPick(event: Event) {
  const input = event.target as HTMLInputElement;
  const files = Array.from(input.files ?? []);
  // Reset first: without this, picking the same file twice fires no change event.
  input.value = '';
  if (!files.length) return;

  failure.value = '';
  const images = files.filter((f) => f.type.startsWith('image/'));
  if (images.length !== files.length) {
    report(`已忽略 ${files.length - images.length} 个非图片文件`);
  }

  const slots = props.maxImages - ids.value.length - jobs.value.length;
  if (slots <= 0) {
    report(`最多只能上传 ${props.maxImages} 张图片`);
    return;
  }
  if (images.length > slots) {
    report(`最多只能上传 ${props.maxImages} 张图片，已选取前 ${slots} 张`);
  }

  for (const file of images.slice(0, slots)) {
    await process(file);
  }
}

async function process(file: File) {
  const job: Job = { key: ++seq, name: file.name, stage: 'compress' };
  jobs.value = [...jobs.value, job];
  const setStage = (stage: Stage) => {
    jobs.value = jobs.value.map((j) => (j.key === job.key ? { ...j, stage } : j));
  };

  try {
    const source = await decode(file);
    const { blob, width, height, mime } = await shrink(source, file);

    setStage('encode');
    const cipher = await props.codecEncode(await toBase64(blob));

    setStage('upload');
    const { id } = await props.uploadFn({
      cipher,
      layout: props.layout,
      mime,
      width,
      height,
      fieldKey: props.fieldKey,
    });

    previews.value[id] = URL.createObjectURL(blob);
    emit('update:modelValue', [...ids.value, id]);
  } catch (err) {
    report(errorMessage(err));
  } finally {
    jobs.value = jobs.value.filter((j) => j.key !== job.key);
  }
}

interface Decoded {
  el: HTMLImageElement;
  url: string;
}

function decode(file: File): Promise<Decoded> {
  return new Promise((resolve, reject) => {
    const url = URL.createObjectURL(file);
    const el = new Image();
    el.onload = () => resolve({ el, url });
    el.onerror = () => {
      URL.revokeObjectURL(url);
      reject(new Error(`无法读取图片「${file.name}」`));
    };
    el.src = url;
  });
}

async function shrink(source: Decoded, file: File) {
  const { el, url } = source;
  try {
    const longest = Math.max(el.naturalWidth, el.naturalHeight);
    const scale = longest > props.maxDimension ? props.maxDimension / longest : 1;
    const width = Math.max(1, Math.round(el.naturalWidth * scale));
    const height = Math.max(1, Math.round(el.naturalHeight * scale));

    const mime = file.type === 'image/png' && file.size < PNG_KEEP_LIMIT ? 'image/png' : 'image/jpeg';

    const canvas = document.createElement('canvas');
    canvas.width = width;
    canvas.height = height;
    const ctx = canvas.getContext('2d');
    if (!ctx) throw new Error('当前浏览器无法处理图片');

    if (mime === 'image/jpeg') {
      // JPEG has no alpha channel: without a backdrop, transparent pixels come
      // out black, which on a white survey form looks like a broken upload.
      ctx.fillStyle = '#ffffff';
      ctx.fillRect(0, 0, width, height);
    }
    ctx.drawImage(el, 0, 0, width, height);

    return { blob: await toBlob(canvas, mime, props.quality), width, height, mime };
  } finally {
    URL.revokeObjectURL(url);
  }
}

function toBlob(canvas: HTMLCanvasElement, mime: string, quality: number): Promise<Blob> {
  return new Promise((resolve, reject) => {
    canvas.toBlob(
      (blob) => (blob ? resolve(blob) : reject(new Error('图片压缩失败'))),
      mime,
      quality,
    );
  });
}

function toBase64(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => {
      const result = String(reader.result);
      const comma = result.indexOf(',');
      resolve(comma >= 0 ? result.slice(comma + 1) : result);
    };
    reader.onerror = () => reject(new Error('图片编码失败'));
    reader.readAsDataURL(blob);
  });
}

function remove(id: number) {
  if (props.disabled) return;
  const url = previews.value[id];
  if (url) {
    URL.revokeObjectURL(url);
    delete previews.value[id];
  }
  emit(
    'update:modelValue',
    ids.value.filter((v) => v !== id),
  );
}

onBeforeUnmount(() => {
  for (const url of Object.values(previews.value)) URL.revokeObjectURL(url);
});
</script>

<template>
  <div class="w-full">
    <input
      ref="fileInput"
      type="file"
      class="sr-only"
      tabindex="-1"
      :accept="accept"
      :multiple="maxImages > 1"
      :disabled="disabled"
      @change="onPick"
    />

    <div class="grid grid-cols-3 gap-2.5 sm:grid-cols-4">
      <div
        v-for="id in ids"
        :key="id"
        class="group relative aspect-square overflow-hidden rounded-[var(--radius-control)] border border-line bg-sunken"
      >
        <img v-if="previews[id]" :src="previews[id]" alt="" class="size-full object-cover" />
        <div v-else class="grid size-full place-items-center text-subtle">
          <ImageIcon class="size-5" aria-hidden="true" />
        </div>
        <button
          type="button"
          class="absolute right-1 top-1 grid size-10 place-items-center rounded-full bg-slate-950/65 text-white backdrop-blur-sm transition-colors duration-200 hover:bg-rose-600 disabled:opacity-50 sm:size-8"
          :disabled="disabled"
          :aria-label="`移除图片 ${id}`"
          @click="remove(id)"
        >
          <X class="size-4" aria-hidden="true" />
        </button>
      </div>

      <div
        v-for="job in jobs"
        :key="`job-${job.key}`"
        class="grid aspect-square place-content-center justify-items-center gap-1.5 rounded-[var(--radius-control)] border border-dashed border-line-strong bg-sunken p-2 text-subtle"
      >
        <Loader2 class="size-5 animate-spin" aria-hidden="true" />
        <span class="text-[11px]">{{ STAGE_LABEL[job.stage] }}</span>
      </div>

      <button
        v-if="!full"
        type="button"
        class="grid aspect-square place-content-center justify-items-center gap-1.5 rounded-[var(--radius-control)] border border-dashed border-line-strong bg-raised text-subtle transition-colors duration-200 hover:border-brand-400 hover:text-brand-600 disabled:cursor-not-allowed disabled:opacity-50 dark:hover:border-brand-400 dark:hover:text-brand-300"
        :disabled="disabled"
        :aria-label="`添加图片，最多 ${maxImages} 张`"
        @click="openPicker"
      >
        <Plus class="size-5" aria-hidden="true" />
        <span class="text-[11px]">添加图片</span>
      </button>
    </div>

    <p v-if="failure" role="alert" class="mt-2 flex items-start gap-1.5 text-xs text-rose-600 dark:text-rose-400">
      <AlertTriangle class="mt-px size-3.5 shrink-0" aria-hidden="true" />
      <span>{{ failure }}</span>
    </p>

    <p v-else class="mt-2 text-xs text-subtle">
      <template v-if="maxImages > 1">已上传 {{ ids.length }}/{{ maxImages }} 张 · </template>
      图片会在本机压缩加密后再上传
    </p>
  </div>
</template>
