/**
 * The 1820 ways to choose 4 clue cells out of 16, mirrored from
 * backend/internal/codec/possets.go.
 *
 * Lexicographic order with the last index varying fastest, so index 0 is
 * {0,1,2,3} and index 1819 is {12,13,14,15}. As with the grid order, this is
 * pinned because it defines the bit positions inside the validity bitmap.
 */

import { sha256, toHex } from './sha256';

export const POS_SET_COUNT = 1820;

export type PosSet = readonly [number, number, number, number];

function build(): PosSet[] {
  const out: PosSet[] = [];
  for (let a = 0; a < 13; a++) {
    for (let b = a + 1; b < 14; b++) {
      for (let c = b + 1; c < 15; c++) {
        for (let d = c + 1; d < 16; d++) {
          out.push([a, b, c, d]);
        }
      }
    }
  }
  return out;
}

let cached: PosSet[] | null = null;
let cachedHash = '';

function ensure(): void {
  if (cached) return;
  cached = build();
  // Go hashes each cell index as a big-endian uint16; a 16-bit index is
  // [0x00, v], which is what this reproduces.
  const flat = new Uint8Array(cached.length * 8);
  let o = 0;
  for (const p of cached) {
    for (const v of p) {
      flat[o++] = 0;
      flat[o++] = v;
    }
  }
  cachedHash = toHex(sha256(flat));
}

export function posSets(): PosSet[] {
  ensure();
  return cached!;
}

export function posSetsSha256(): string {
  ensure();
  return cachedHash;
}
