/**
 * The browser's side of the sudoku codec.
 *
 * Responsibilities:
 *   - load and cryptographically verify the shared validity bitmap;
 *   - derive a table per questionnaire from the derived key the API hands out;
 *   - encode submissions (off the main thread for large payloads);
 *   - verify the server agrees on the derived table before anything is sent.
 */

import { defineStore } from 'pinia';
import { ref, shallowRef } from 'vue';

import { Codec } from '@/codec';
import { decode, encode } from '@/codec/classic';
import { LayoutId } from '@/codec/layout';
import { PINNED_GRIDS_SHA256, PINNED_POSSETS_SHA256, PINNED_VALIDITY_SHA256, VALIDITY_URL } from '@/codec/pins';
import { PRNG } from '@/codec/prng';
import { gridsSha256 } from '@/codec/grids';
import { posSetsSha256 } from '@/codec/possets';
import { sha256, toHex } from '@/codec/sha256';
import { Validity } from '@/codec/validity';
import { textToBytes, utf8Decode, utf8Encode } from '@/codec';
import type { WorkerRequest, WorkerResponse } from '@/codec/worker';

export type CodecStatus = 'idle' | 'loading' | 'ready' | 'error';

/** Payloads above this size are encoded in a worker. */
const WORKER_THRESHOLD_BYTES = 32 * 1024;

export const useCodecStore = defineStore('codec', () => {
  const status = ref<CodecStatus>('idle');
  const error = ref('');
  const codec = shallowRef<Codec | null>(null);

  /**
   * Fingerprint mismatches actually observed. These are surfaced in the UI
   * rather than thrown: a mismatch means the browser and server disagree, so
   * the user must be told explicitly instead of getting corrupt data.
   */
  const mismatches = ref<string[]>([]);

  let loading: Promise<Codec> | null = null;
  let worker: Worker | null = null;
  let workerSeq = 0;
  const pending = new Map<number, { resolve: (b: Uint8Array) => void; reject: (e: Error) => void }>();

  function getWorker(): Worker | null {
    if (worker) return worker;
    if (typeof Worker === 'undefined') return null;
    try {
      worker = new Worker(new URL('../codec/worker.ts', import.meta.url), { type: 'module' });
      worker.onmessage = (event: MessageEvent<WorkerResponse>) => {
        const entry = pending.get(event.data.id);
        if (!entry) return;
        pending.delete(event.data.id);
        if (event.data.ok && event.data.bytes) entry.resolve(event.data.bytes);
        else entry.reject(new Error(event.data.error || '编码失败'));
      };
      worker.onerror = () => {
        for (const [, entry] of pending) entry.reject(new Error('编码线程异常'));
        pending.clear();
        worker = null;
      };
    } catch {
      worker = null;
    }
    return worker;
  }

  /** Loads the bitmap, verifying every digest along the way. */
  async function load(): Promise<Codec> {
    if (codec.value) return codec.value;
    if (loading) return loading;

    status.value = 'loading';
    loading = (async () => {
      // Cheap sanity checks first: these run in milliseconds and catch an
      // enumeration drift before we even fetch the bitmap.
      if (gridsSha256() !== PINNED_GRIDS_SHA256) {
        throw new Error('网格枚举与固定值不一致');
      }
      if (posSetsSha256() !== PINNED_POSSETS_SHA256) {
        throw new Error('位置组合枚举与固定值不一致');
      }

      const response = await fetch(VALIDITY_URL, { cache: 'force-cache' });
      if (!response.ok) throw new Error(`无法加载数独校验位图(${response.status})`);
      const bytes = new Uint8Array(await response.arrayBuffer());
      if (toHex(sha256(bytes)) !== PINNED_VALIDITY_SHA256) {
        throw new Error('数独校验位图校验失败');
      }
      const instance = new Codec(new Validity(bytes));
      codec.value = instance;
      status.value = 'ready';
      return instance;
    })();

    try {
      return await loading;
    } catch (err) {
      status.value = 'error';
      error.value = err instanceof Error ? err.message : String(err);
      loading = null;
      throw err;
    }
  }

  /** Derives a questionnaire's table, then checks it against the server's. */
  async function prepare(tableKey: string, layout: LayoutId, serverFingerprint: string): Promise<void> {
    const instance = await load();
    const local = instance.fingerprint(tableKey, layout);
    if (serverFingerprint && local !== serverFingerprint) {
      const note = `编码表指纹不一致:本地 ${local.slice(0, 12)}… / 服务端 ${serverFingerprint.slice(0, 12)}…`;
      if (!mismatches.value.includes(note)) mismatches.value.push(note);
      throw new Error(note);
    }
  }

  async function runInWorker(op: 'encode' | 'decode', tableKey: string, layout: LayoutId, bytes: Uint8Array): Promise<Uint8Array> {
    const w = getWorker();
    if (!w) throw new Error('no-worker');
    const id = ++workerSeq;
    return new Promise<Uint8Array>((resolve, reject) => {
      pending.set(id, { resolve, reject });
      const request: WorkerRequest = { id, op, tableKey, layout, bytes };
      w.postMessage(request, [bytes.buffer]);
    });
  }

  /** Encodes arbitrary bytes into ciphertext. */
  async function encodeBytes(bytes: Uint8Array, tableKey: string, layout: LayoutId = 'ascii'): Promise<string> {
    const instance = await load();
    if (bytes.length >= WORKER_THRESHOLD_BYTES) {
      try {
        const out = await runInWorker('encode', tableKey, layout, bytes);
        return latin1(out);
      } catch {
        // Any worker failure falls back to the main thread rather than losing
        // the submission; it is slower, not wrong.
      }
    }
    const out = encode(instance.table(tableKey, layout), PRNG.random(), bytes, 0);
    return latin1(out);
  }

  async function decodeBytes(cipher: string, tableKey: string, layout: LayoutId = 'ascii'): Promise<Uint8Array> {
    const instance = await load();
    const bytes = textToBytes(cipher);
    if (bytes.length >= WORKER_THRESHOLD_BYTES) {
      try {
        return await runInWorker('decode', tableKey, layout, bytes);
      } catch {
        /* fall through */
      }
    }
    return decode(instance.table(tableKey, layout), bytes);
  }

  /** Encodes a JSON value into ciphertext text. */
  async function encodeJson(value: unknown, tableKey: string, layout: LayoutId = 'ascii'): Promise<string> {
    return encodeBytes(utf8Encode(JSON.stringify(value ?? null)), tableKey, layout);
  }

  /** Decodes ciphertext text into a JSON value. */
  async function decodeJson<T>(cipher: string, tableKey: string, layout: LayoutId = 'ascii'): Promise<T> {
    return JSON.parse(utf8Decode(await decodeBytes(cipher, tableKey, layout))) as T;
  }

  /** Encoding counts per byte value, for the inspector's explanation panel. */
  function tableStats(tableKey: string, layout: LayoutId = 'ascii') {
    const instance = codec.value;
    if (!instance) return null;
    const t = instance.table(tableKey, layout);
    let min = Number.MAX_SAFE_INTEGER;
    let max = 0;
    for (let b = 0; b < 256; b++) {
      const n = t.encodingCount(b);
      if (n < min) min = n;
      if (n > max) max = n;
    }
    return { min, max, distinctPuzzles: t.decodeMapSize(), fingerprint: t.fingerprint };
  }

  load().catch(() => {
    /* the UI renders the error state from `status` */
  });

  return {
    status, error, codec, mismatches,
    load, prepare, encodeBytes, decodeBytes, encodeJson, decodeJson, tableStats,
  };
});

function latin1(bytes: Uint8Array): string {
  const CHUNK = 8192;
  const parts: string[] = [];
  for (let i = 0; i < bytes.length; i += CHUNK) {
    parts.push(String.fromCharCode(...bytes.subarray(i, Math.min(i + CHUNK, bytes.length))));
  }
  return parts.join('');
}
