/**
 * Codec Web Worker.
 *
 * Encoding a photo payload expands it ~4x and takes tens to hundreds of
 * milliseconds, which would visibly stall the form on a phone. Running it here
 * keeps the main thread at 60fps while a submission is prepared.
 */

import { decode, encode } from './classic';
import { Codec } from './index';
import { LayoutId } from './layout';
import { PINNED_VALIDITY_SHA256, VALIDITY_URL } from './pins';
import { PRNG } from './prng';
import { sha256, toHex } from './sha256';
import { Validity } from './validity';

export interface WorkerRequest {
  id: number;
  op: 'encode' | 'decode';
  tableKey: string;
  layout: LayoutId;
  bytes: Uint8Array;
}

export interface WorkerResponse {
  id: number;
  ok: boolean;
  bytes?: Uint8Array;
  error?: string;
}

let codecPromise: Promise<Codec> | null = null;

/** Loads and verifies the shared bitmap before deriving any table from it. */
function getCodec(): Promise<Codec> {
  if (!codecPromise) {
    codecPromise = (async () => {
      const response = await fetch(VALIDITY_URL, { cache: 'force-cache' });
      if (!response.ok) throw new Error(`无法加载数独校验位图(${response.status})`);
      const bytes = new Uint8Array(await response.arrayBuffer());
      if (toHex(sha256(bytes)) !== PINNED_VALIDITY_SHA256) {
        throw new Error('数独校验位图校验失败,前端与服务端的编码表可能不一致');
      }
      return new Codec(new Validity(bytes));
    })();
  }
  return codecPromise;
}

self.onmessage = async (event: MessageEvent<WorkerRequest>) => {
  const { id, op, tableKey, layout, bytes } = event.data;
  const reply: WorkerResponse = { id, ok: false };
  try {
    const codec = await getCodec();
    const table = codec.table(tableKey, layout);
    reply.bytes =
      op === 'encode' ? encode(table, PRNG.random(), bytes, 0) : decode(table, bytes);
    reply.ok = true;
  } catch (err) {
    reply.error = err instanceof Error ? err.message : String(err);
  }
  (self as unknown as { postMessage: (msg: WorkerResponse) => void }).postMessage(reply);
};
