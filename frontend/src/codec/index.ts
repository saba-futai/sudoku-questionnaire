/**
 * Public surface of the sudoku codec. See spec/CODEC.md for the normative
 * specification and backend/internal/codec for the Go mirror.
 */

export * from './sha256';
export * from './prng';
export * from './grids';
export * from './possets';
export * from './validity';
export * from './layout';
export * from './table';
export * from './classic';
export * from './crc32';

import { PRNG } from './prng';
import { Validity } from './validity';
import { buildTable, Table } from './table';
import { LayoutId } from './layout';
import { decode, encode, MAX_PAD_DEFAULT } from './classic';

/**
 * Per-questionnaire key derivation.
 *
 * The master key never leaves the server; the API hands the browser the derived
 * value, so compromising one questionnaire's key does not expose any other
 * questionnaire's data. The browser therefore never needs this function at run
 * time — it receives the derived key — but it is kept here so tests and tooling
 * can reproduce the server's derivation exactly.
 */
export function deriveKey(masterKey: string, questionnaireId: number | string): string {
  return `${masterKey}|q:${questionnaireId}`;
}

/**
 * Byte-exact text conversion for ciphertext.
 *
 * Deliberately latin1 rather than UTF-8: with the ASCII layout every ciphertext
 * byte is already < 0x80 so the two agree, but the entropy layout emits bytes
 * >= 0x80 and UTF-8 would mangle them. Round-tripping byte-for-byte is what
 * matters, not the string's visual meaning.
 */
export function bytesToText(bytes: Uint8Array): string {
  const CHUNK = 8192;
  const parts: string[] = [];
  for (let i = 0; i < bytes.length; i += CHUNK) {
    parts.push(String.fromCharCode(...bytes.subarray(i, Math.min(i + CHUNK, bytes.length))));
  }
  return parts.join('');
}

export function textToBytes(text: string): Uint8Array {
  const out = new Uint8Array(text.length);
  for (let i = 0; i < text.length; i++) out[i] = text.charCodeAt(i) & 0xff;
  return out;
}

const textEncoder = new TextEncoder();
const textDecoder = new TextDecoder();

export const utf8Encode = (s: string): Uint8Array => textEncoder.encode(s);
export const utf8Decode = (b: Uint8Array): string => textDecoder.decode(b);

/**
 * Caches derived tables per key. Building one costs a few milliseconds, so each
 * distinct key pays it once per session.
 *
 * The codec keys off the DERIVED key the API hands out, not a master key: the
 * browser never holds the master secret.
 */
export class Codec {
  private readonly tables = new Map<string, Table>();

  constructor(private readonly validity: Validity) {}

  table(tableKey: string, layoutId: LayoutId = 'ascii'): Table {
    const cacheKey = `${tableKey}\u0000${layoutId}`;
    let t = this.tables.get(cacheKey);
    if (!t) {
      t = buildTable(tableKey, layoutId, this.validity);
      this.tables.set(cacheKey, t);
    }
    return t;
  }

  /** Encodes JSON-serialisable data and returns the ciphertext as text. */
  encodeJson(value: unknown, tableKey: string, layoutId: LayoutId = 'ascii', maxPad = MAX_PAD_DEFAULT): string {
    const plain = utf8Encode(JSON.stringify(value ?? null));
    return bytesToText(encode(this.table(tableKey, layoutId), PRNG.random(), plain, maxPad));
  }

  /** Decodes ciphertext text back into the original JSON value. */
  decodeJson<T = unknown>(cipher: string, tableKey: string, layoutId: LayoutId = 'ascii'): T {
    const plain = decode(this.table(tableKey, layoutId), textToBytes(cipher));
    return JSON.parse(utf8Decode(plain)) as T;
  }

  /** Encodes raw bytes (used for base64 image payloads). */
  encodeBytes(bytes: Uint8Array, tableKey: string, layoutId: LayoutId = 'ascii', maxPad = MAX_PAD_DEFAULT): string {
    return bytesToText(encode(this.table(tableKey, layoutId), PRNG.random(), bytes, maxPad));
  }

  decodeBytes(cipher: string, tableKey: string, layoutId: LayoutId = 'ascii'): Uint8Array {
    return decode(this.table(tableKey, layoutId), textToBytes(cipher));
  }

  /** Fingerprint of a derived table, for the client-side integrity check. */
  fingerprint(tableKey: string, layoutId: LayoutId = 'ascii'): string {
    return this.table(tableKey, layoutId).fingerprint;
  }
}
