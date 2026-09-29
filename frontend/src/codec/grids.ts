/**
 * The 288 valid 4x4 sudoku grids, mirrored from backend/internal/codec/grids.go.
 *
 * The enumeration ORDER is load-bearing: the per-key shuffle permutes indices
 * into this array, so a different traversal yields 288 correct-but-reordered
 * grids and therefore a completely different table. The hash of this exact
 * sequence is pinned in spec/vectors/structure.json and asserted at boot.
 */

import { sha256, toHex } from './sha256';

export const GRID_COUNT = 288;

/** Row-major: index = row*4 + col, values 1..4. */
export type Grid = Uint8Array;

function generate(): Grid[] {
  const out: Grid[] = [];
  const g = new Uint8Array(16);

  const backtrack = (idx: number): void => {
    if (idx === 16) {
      out.push(Uint8Array.from(g));
      return;
    }
    const row = (idx / 4) | 0;
    const col = idx % 4;
    const br = ((row / 2) | 0) * 2;
    const bc = ((col / 2) | 0) * 2;
    for (let num = 1; num <= 4; num++) {
      let valid = true;
      for (let i = 0; i < 4; i++) {
        if (g[row * 4 + i] === num || g[i * 4 + col] === num) {
          valid = false;
          break;
        }
      }
      if (valid) {
        for (let r = 0; r < 2 && valid; r++) {
          for (let c = 0; c < 2; c++) {
            if (g[(br + r) * 4 + (bc + c)] === num) {
              valid = false;
              break;
            }
          }
        }
      }
      if (valid) {
        g[idx] = num;
        backtrack(idx + 1);
        g[idx] = 0;
      }
    }
  };

  backtrack(0);
  return out;
}

let cached: Grid[] | null = null;
let cachedHash = '';

function ensure(): void {
  if (cached) return;
  cached = generate();
  const flat = new Uint8Array(cached.length * 16);
  cached.forEach((g, i) => flat.set(g, i * 16));
  cachedHash = toHex(sha256(flat));
}

export function grids(): Grid[] {
  ensure();
  return cached!;
}

/** Hex digest over the 288 grids in enumeration order. */
export function gridsSha256(): string {
  ensure();
  return cachedHash;
}
