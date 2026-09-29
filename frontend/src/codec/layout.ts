/**
 * Wire layouts, mirrored from backend/internal/codec/layout.go.
 *
 * A layout is a bijection between the 64 group values and 64 wire bytes, plus a
 * disjoint padding pool. Because groups — not wire bytes — are what the decode
 * map is keyed on, one table serves both layouts.
 */

export type LayoutId = 'ascii' | 'entropy';

export function parseLayoutId(s: string | undefined | null): LayoutId {
  return s === 'entropy' ? 'entropy' : 'ascii';
}

export class Layout {
  /** group -> wire byte */
  readonly hint = new Uint8Array(64);
  /** wire byte -> group (meaningful only where isHint is 1) */
  readonly group = new Uint8Array(256);
  /** 1 where the byte is in the clue alphabet */
  readonly isHint = new Uint8Array(256);
  /** 1 where the byte is filler */
  readonly isPad = new Uint8Array(256);
  readonly padding: Uint8Array;

  constructor(readonly id: LayoutId) {
    if (id === 'entropy') {
      for (let g = 0; g < 64; g++) {
        const b = (((g & 0x30) << 1) | (g & 0x0f)) & 0xff;
        this.hint[g] = b;
        this.isHint[b] = 1;
        this.group[b] = g;
      }
      const pool: number[] = [];
      for (let i = 0; i < 8; i++) pool.push(0x80 + i, 0x10 + i);
      this.padding = Uint8Array.from(pool);
    } else {
      for (let g = 0; g < 64; g++) {
        let b = 0x40 | g;
        // 0x7F is DEL, not printable. The alphabet needs 64 symbols and
        // 0x40..0x7E supplies only 63, so the 64th borrows '\n'. Safe here
        // because this ciphertext only ever travels in SQLite TEXT and JSON,
        // both of which carry '\n' exactly.
        if (b === 0x7f) b = 0x0a;
        this.hint[g] = b;
        this.isHint[b] = 1;
        this.group[b] = g;
      }
      const pool: number[] = [];
      for (let i = 0; i < 32; i++) pool.push(0x20 + i);
      this.padding = Uint8Array.from(pool);
    }
    for (const p of this.padding) this.isPad[p] = 1;
  }

  hintByte(g: number): number {
    return this.hint[g & 0x3f];
  }

  /**
   * Maps a wire byte back to its group, or -1.
   *
   * Exact membership, deliberately stricter than upstream's
   * `(b & 0x40) == 0x40`, which also accepts 0x80..0xFF and would silently
   * swallow any stray UTF-8 text as clues.
   */
  groupOf(b: number): number {
    return this.isHint[b] ? this.group[b] : -1;
  }

  isPadding(b: number): boolean {
    return this.isPad[b] === 1;
  }

  /** Startup assertion: a collision here would make decode ambiguous. */
  validate(): void {
    const seen = new Map<number, number>();
    for (let g = 0; g < 64; g++) {
      const b = this.hint[g];
      const prev = seen.get(b);
      if (prev !== undefined) {
        throw new Error(`layout ${this.id}: wire byte 0x${b.toString(16)} used by groups ${prev} and ${g}`);
      }
      seen.set(b, g);
      if (this.isHint[b] !== 1 || this.group[b] !== g) {
        throw new Error(`layout ${this.id}: group round-trip failed for ${g}`);
      }
    }
    for (const p of this.padding) {
      if (this.isHint[p] === 1) {
        throw new Error(`layout ${this.id}: padding byte 0x${p.toString(16)} is also a clue byte`);
      }
    }
  }
}

const cache = new Map<LayoutId, Layout>();

export function layout(id: LayoutId): Layout {
  let l = cache.get(id);
  if (!l) {
    l = new Layout(id);
    cache.set(id, l);
  }
  return l;
}
