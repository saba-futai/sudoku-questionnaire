import type { Question } from '@/api/types';

/** Formats an ISO timestamp for display, tolerant of missing values. */
export function formatDate(iso: string | null | undefined, withTime = true): string {
  if (!iso) return '—';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '—';
  const pad = (n: number) => String(n).padStart(2, '0');
  const date = `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
  if (!withTime) return date;
  return `${date} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

/** Relative time, falling back to an absolute date beyond a month. */
export function relativeTime(iso: string | null | undefined): string {
  if (!iso) return '—';
  const then = new Date(iso).getTime();
  if (Number.isNaN(then)) return '—';
  const diff = Date.now() - then;
  const min = Math.round(diff / 60000);
  if (min < 1) return '刚刚';
  if (min < 60) return `${min} 分钟前`;
  const hours = Math.round(min / 60);
  if (hours < 24) return `${hours} 小时前`;
  const days = Math.round(hours / 24);
  if (days < 31) return `${days} 天前`;
  return formatDate(iso, false);
}

export function formatDuration(ms: number): string {
  if (!ms || ms <= 0) return '—';
  const total = Math.round(ms / 1000);
  if (total < 60) return `${total} 秒`;
  const m = Math.floor(total / 60);
  const s = total % 60;
  return s ? `${m} 分 ${s} 秒` : `${m} 分`;
}

export function formatBytes(bytes: number): string {
  if (!bytes) return '0 B';
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}

export function formatNumber(n: number): string {
  return n.toLocaleString('zh-CN');
}

/** Human label for a question type. */
export const QUESTION_TYPE_LABELS: Record<Question['type'], string> = {
  single: '单选题',
  multi: '多选题',
  select: '下拉选择',
  text: '单行文本',
  textarea: '多行文本',
  rating: '评分',
  date: '日期',
  image: '图片上传',
};

export function typeLabel(t: Question['type']): string {
  return QUESTION_TYPE_LABELS[t] ?? t;
}

/**
 * Renders an answer for display, mapping option values to their labels.
 * Used by the response table, the detail drawer and the export preview.
 */
export function displayAnswer(question: Question | undefined, value: unknown): string {
  if (value === null || value === undefined || value === '') return '';
  const label = (v: string): string =>
    question?.options?.find((o) => o.value === v)?.label ?? v;

  if (Array.isArray(value)) {
    return value.map((v) => (typeof v === 'string' ? label(v) : String(v))).join('、');
  }
  if (typeof value === 'string') {
    if (question?.type === 'single' || question?.type === 'select') return label(value);
    return value;
  }
  return String(value);
}

export function isAnswered(value: unknown): boolean {
  if (value === null || value === undefined) return false;
  if (typeof value === 'string') return value.trim() !== '';
  if (Array.isArray(value)) return value.length > 0;
  return true;
}

/** True when the answer payload could not be decoded server-side. */
export function hasDecodeError(answers: Record<string, unknown>): boolean {
  return typeof answers?.__error === 'string';
}

export function decodeError(answers: Record<string, unknown>): string {
  return String(answers?.__error ?? '');
}

export function slugifyPreview(slug: string): string {
  return `${window.location.origin}/q/${slug}`;
}
