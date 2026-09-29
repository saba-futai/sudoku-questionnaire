/**
 * The validity bitmap, mirrored from backend/internal/codec/validity.go.
 *
 * Key property: whether a clue set uniquely identifies a grid does NOT depend
 * on the encryption key — only on the two fixed enumerations. The expensive
 * 288 x 1820 signature computation is therefore a build-time artifact shipped
 * as a static asset, and the per-questionnaire work is just a 288-element
 * shuffle plus a map build.
 *
 * `computeValidity()` is still implemented in full so the browser remains a
 * complete implementation of the spec rather than a consumer of server
 * artifacts, and a test asserts it equals the shipped bitmap byte for byte.
 */

import { grids } from './grids';
import { posSets } from './possets';

export const VALIDITY_ROW_BYTES = 228;
export const VALIDITY_BYTES = 288 * VALIDITY_ROW_BYTES; // 65664

/** A clue as a 6-bit group: digit-1 in bits 4-5, cell index in bits 0-3. */
export function group(digit: number, pos: number): number {
  return ((digit << 4) | (pos & 0x0f)) & 0x3f;
}

/**
 * Sorts four groups ascending and packs them into 24 bits.
 *
 * The sort is what makes the decode key order-independent, which is exactly
 * what lets the encoder emit the four clue bytes in any of 24 permutations.
 * Packing 6-bit fields rather than raw wire bytes keeps the result below 2^31,
 * so `<<` never produces a negative number here.
 */
export function packGroups(g0: number, g1: number, g2: number, g3: number): number {
  let a = g0;
  let b = g1;
  let c = g2;
  let d = g3;
  // 5-comparator sorting network: branch-free and identical in both languages.
  if (a > b) { const t = a; a = b; b = t; }
  if (c > d) { const t = c; c = d; d = t; }
  if (a > c) { const t = a; a = c; c = t; }
  if (b > d) { const t = b; b = d; d = t; }
  if (b > c) { const t = b; b = c; c = t; }
  return ((a << 18) | (b << 12) | (c << 6) | d) >>> 0;
}

export class Validity {
  constructor(readonly bits: Uint8Array) {
    if (bits.length !== VALIDITY_BYTES) {
      throw new Error(`validity bitmap: ${bits.length} bytes, want ${VALIDITY_BYTES}`);
    }
  }

  valid(g: number, p: number): boolean {
    return (this.bits[g * VALIDITY_ROW_BYTES + (p >> 3)] & (1 << (p & 7))) !== 0;
  }

  encodeCount(g: number): number {
    let n = 0;
    const base = g * VALIDITY_ROW_BYTES;
    for (let i = 0; i < VALIDITY_ROW_BYTES; i++) {
      let v = this.bits[base + i];
      while (v) {
        n += v & 1;
        v >>= 1;
      }
    }
    return n;
  }

  counts(): number[] {
    const out = new Array<number>(288);
    for (let g = 0; g < 288; g++) out[g] = this.encodeCount(g);
    return out;
  }
}

/** Reference implementation: derives the bitmap from scratch in ~200-400ms. */
export function computeValidity(): Validity {
  const gs = grids();
  const ps = posSets();

  // Pass 1: how many (grid, position-set) pairs share each signature. A
  // signature belongs to exactly one grid iff this count is 1.
  const counts = new Map<number, number>();
  for (let gi = 0; gi < gs.length; gi++) {
    const g = gs[gi];
    for (let pi = 0; pi < ps.length; pi++) {
      const p = ps[pi];
      const sig = packGroups(
        group(g[p[0]] - 1, p[0]),
        group(g[p[1]] - 1, p[1]),
        group(g[p[2]] - 1, p[2]),
        group(g[p[3]] - 1, p[3]),
      );
      counts.set(sig, (counts.get(sig) ?? 0) + 1);
    }
  }

  // Pass 2: record the uniquely-identifying position sets.
  const bits = new Uint8Array(VALIDITY_BYTES);
  for (let gi = 0; gi < gs.length; gi++) {
    const g = gs[gi];
    for (let pi = 0; pi < ps.length; pi++) {
      const p = ps[pi];
      const sig = packGroups(
        group(g[p[0]] - 1, p[0]),
        group(g[p[1]] - 1, p[1]),
        group(g[p[2]] - 1, p[2]),
        group(g[p[3]] - 1, p[3]),
      );
      if (counts.get(sig) === 1) {
        bits[gi * VALIDITY_ROW_BYTES + (pi >> 3)] |= 1 << (pi & 7);
      }
    }
  }
  return new Validity(bits);
}
