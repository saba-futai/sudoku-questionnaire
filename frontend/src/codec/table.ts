/**
 * The keyed byte -> puzzle mapping, mirrored from backend/internal/codec/table.go.
 *
 * Built per questionnaire in a few milliseconds: shuffle the 288 grid indices,
 * then inherit each byte's valid clue sets from the validity bitmap.
 */

import { sha256, toHex } from './sha256';
import { PRNG, tableSeedLoHi } from './prng';
import { grids, GRID_COUNT } from './grids';
import { posSets } from './possets';
import { group, packGroups, Validity } from './validity';
import { Layout, LayoutId, layout as getLayout } from './layout';

/** All 24 permutations of {0,1,2,3}, lexicographic. Mirrors Go's perm4. */
export const PERM4: readonly (readonly [number, number, number, number])[] = (() => {
  const out: [number, number, number, number][] = [];
  for (let a = 0; a < 4; a++) {
    for (let b = 0; b < 4; b++) {
      if (b === a) continue;
      for (let c = 0; c < 4; c++) {
        if (c === a || c === b) continue;
        for (let d = 0; d < 4; d++) {
          if (d === a || d === b || d === c) continue;
          out.push([a, b, c, d]);
        }
      }
    }
  }
  return out;
})();

export class Table {
  readonly fingerprint: string;
  readonly layout: Layout;

  /**
   * All clue quadruples, 4 bytes per entry, concatenated in plaintext-byte
   * order. Public because the encoder reads it in its innermost loop and any
   * accessor returning a tuple would allocate once per plaintext byte.
   */
  readonly quads: Uint8Array;
  /** Prefix offsets into `quads`, in quadruple units. Length 257. */
  readonly offsets: Int32Array;
  private readonly decode: Map<number, number>;

  constructor(
    readonly key: string,
    layoutId: LayoutId,
    quads: Uint8Array,
    offsets: Int32Array,
    decode: Map<number, number>,
    fingerprint: string,
  ) {
    this.layout = getLayout(layoutId);
    this.quads = quads;
    this.offsets = offsets;
    this.decode = decode;
    this.fingerprint = fingerprint;
  }

  encodingCount(b: number): number {
    return this.offsets[b + 1] - this.offsets[b];
  }

  /**
   * Byte offset of a randomly chosen quadruple for plaintext byte b.
   * Callers read four group bytes starting there.
   */
  pickQuad(b: number, rng: PRNG): number {
    const start = this.offsets[b];
    const count = this.offsets[b + 1] - start;
    return (start + rng.nextBelow(count)) * 4;
  }

  lookup(g0: number, g1: number, g2: number, g3: number): number {
    return this.decode.get(packGroups(g0, g1, g2, g3)) ?? -1;
  }

  decodeMapSize(): number {
    return this.decode.size;
  }

  /** Reads the clue quadruple at a quadruple index, for the codec inspector. */
  quad(i: number): [number, number, number, number] {
    const o = i * 4;
    return [this.quads[o], this.quads[o + 1], this.quads[o + 2], this.quads[o + 3]];
  }
}

export function buildTable(key: string, layoutId: LayoutId, v: Validity): Table {
  const lay = getLayout(layoutId);
  lay.validate();

  const order = new Array<number>(GRID_COUNT);
  for (let i = 0; i < GRID_COUNT; i++) order[i] = i;
  PRNG.fromSeed(tableSeedLoHi(key)).shuffle(order);

  const gs = grids();
  const ps = posSets();

  // First pass sizes the flat buffer exactly, avoiding a growing array of
  // per-byte small arrays (which dominates build time otherwise).
  const counts = new Int32Array(256);
  for (let b = 0; b < 256; b++) {
    const gi = order[b];
    let n = 0;
    for (let pi = 0; pi < ps.length; pi++) {
      if (v.valid(gi, pi)) n++;
    }
    counts[b] = n;
  }
  const offsets = new Int32Array(257);
  for (let b = 0; b < 256; b++) offsets[b + 1] = offsets[b] + counts[b];

  const quads = new Uint8Array(offsets[256] * 4);
  const decode = new Map<number, number>();
  for (let b = 0; b < 256; b++) {
    const g = gs[order[b]];
    let o = offsets[b] * 4;
    for (let pi = 0; pi < ps.length; pi++) {
      if (!v.valid(order[b], pi)) continue;
      const p = ps[pi];
      const g0 = group(g[p[0]] - 1, p[0]);
      const g1 = group(g[p[1]] - 1, p[1]);
      const g2 = group(g[p[2]] - 1, p[2]);
      const g3 = group(g[p[3]] - 1, p[3]);
      quads[o++] = g0;
      quads[o++] = g1;
      quads[o++] = g2;
      quads[o++] = g3;
      decode.set(packGroups(g0, g1, g2, g3), b);
    }
    if (counts[b] === 0) {
      throw new Error(`codec: byte ${b} has no valid encoding under key ${JSON.stringify(key)}`);
    }
  }

  return new Table(key, layoutId, quads, offsets, decode, fingerprintOf(quads, offsets));
}

/**
 * Hashes the encode table canonically; layout-independent by construction.
 *
 * MUST interleave each byte's big-endian count with that byte's quadruples, in
 * byte order — that is the exact stream Go hashes. Batching all the counts
 * first produces a different (and silently wrong) digest.
 */
function fingerprintOf(quads: Uint8Array, offsets: Int32Array): string {
  const buf = new Uint8Array(512 + quads.length);
  let o = 0;
  for (let b = 0; b < 256; b++) {
    const n = offsets[b + 1] - offsets[b];
    buf[o++] = (n >> 8) & 0xff;
    buf[o++] = n & 0xff;
    const start = offsets[b] * 4;
    const end = offsets[b + 1] * 4;
    buf.set(quads.subarray(start, end), o);
    o += end - start;
  }
  return toHex(sha256(buf));
}
