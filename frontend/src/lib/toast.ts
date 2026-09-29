import { ref } from 'vue';

export type ToastKind = 'success' | 'error' | 'info';

export interface Toast {
  id: number;
  kind: ToastKind;
  title: string;
  detail?: string;
}

const TOAST_TTL = 4200;

export const toasts = ref<Toast[]>([]);
let seq = 0;

function push(kind: ToastKind, title: string, detail?: string): number {
  const id = ++seq;
  toasts.value = [...toasts.value, { id, kind, title, detail }];
  window.setTimeout(() => dismiss(id), TOAST_TTL);
  return id;
}

export function dismiss(id: number): void {
  toasts.value = toasts.value.filter((t) => t.id !== id);
}

export const toast = {
  success: (title: string, detail?: string) => push('success', title, detail),
  error: (title: string, detail?: string) => push('error', title, detail),
  info: (title: string, detail?: string) => push('info', title, detail),
  dismiss,
};

/** Turns any thrown value into a readable message for a toast. */
export function errorMessage(err: unknown): string {
  if (err instanceof Error) return err.message;
  if (typeof err === 'string') return err;
  return '发生未知错误';
}

export function toastError(err: unknown, title = '操作失败'): void {
  toast.error(title, errorMessage(err));
}
