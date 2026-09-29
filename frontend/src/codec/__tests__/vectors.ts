import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

export const REPO_ROOT = fileURLToPath(new URL('../../../../', import.meta.url));
export const VECTOR_DIR = `${REPO_ROOT}spec/vectors/`;

export function vector<T>(name: string): T {
  return JSON.parse(readFileSync(`${VECTOR_DIR}${name}.json`, 'utf8')) as T;
}

/** Reads the Go-embedded bitmap artifact, which the frontend ships verbatim. */
export function validityArtifact(): Uint8Array {
  return new Uint8Array(readFileSync(`${REPO_ROOT}backend/internal/codec/tables/validity.bin`));
}

export function hexToBytes(hex: string): Uint8Array {
  const out = new Uint8Array(hex.length >> 1);
  for (let i = 0; i < out.length; i++) out[i] = parseInt(hex.substr(i * 2, 2), 16);
  return out;
}

export function bytesToHex(b: Uint8Array): string {
  let s = '';
  for (const x of b) s += x.toString(16).padStart(2, '0');
  return s;
}

/** Splits a 16-hex-digit u64 into splitmix64's (lo, hi) halves. */
export function seedParts(hex: string): [number, number] {
  const padded = hex.padStart(16, '0');
  return [parseInt(padded.slice(8, 16), 16), parseInt(padded.slice(0, 8), 16)];
}

/** Deterministic pseudo-random bytes, for the differential fuzz cases. */
export function pseudoRandomBytes(n: number, seed: number): Uint8Array {
  const out = new Uint8Array(n);
  let x = seed >>> 0;
  for (let i = 0; i < n; i++) {
    x = (Math.imul(x, 1664525) + 1013904223) >>> 0;
    out[i] = x >>> 24;
  }
  return out;
}
