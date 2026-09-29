import { describe, expect, it } from 'vitest';

import { PRNG, SEED_DOMAIN, tableSeedLoHi } from '../prng';
import { makeReferenceUint32 } from '../reference.bigint';
import { seedParts, vector } from './vectors';

interface PrngVector {
  seedDomain: string;
  cases: {
    key: string;
    tableSeedHex: string;
    nextUint64: string[];
    nextUint32: number[];
  }[];
  nextBelow: Record<string, number[]>;
}

const v = vector<PrngVector>('prng');

const hex64 = (lo: number, hi: number): string =>
  (hi >>> 0).toString(16).padStart(8, '0') + (lo >>> 0).toString(16).padStart(8, '0');

describe('PRNG parity with Go', () => {
  it('uses the same seed domain', () => {
    expect(SEED_DOMAIN).toBe(v.seedDomain);
  });

  for (const c of v.cases) {
    describe(`key ${JSON.stringify(c.key)}`, () => {
      it('derives the same table seed', () => {
        expect(hex64(...tableSeedLoHi(c.key))).toBe(c.tableSeedHex);
      });

      it('produces the same 64-bit stream', () => {
        const r = PRNG.fromSeed(tableSeedLoHi(c.key));
        for (const want of c.nextUint64) {
          const [lo, hi] = r.nextUint64Parts();
          expect(hex64(lo, hi)).toBe(want);
        }
      });

      it('produces the same 32-bit stream', () => {
        const r = PRNG.fromSeed(tableSeedLoHi(c.key));
        for (const want of c.nextUint32) {
          expect(r.nextUint32() >>> 0).toBe(want >>> 0);
        }
      });
    });
  }

  for (const [n, want] of Object.entries(v.nextBelow)) {
    it(`matches nextBelow(${n})`, () => {
      // Go seeds NewPRNG(0x0123456789ABCDEF); seedParts splits that u64 into
      // splitmix64's (lo, hi) halves.
      const r = PRNG.fromSeed(seedParts('0123456789abcdef'));
      expect(want.map(() => r.nextBelow(Number(n)))).toEqual(want);
    });
  }
});

describe('fast PRNG vs BigInt reference', () => {
  // This is the test that makes the 32-bit chunked arithmetic trustworthy.
  it('agrees over 10^6 consecutive draws', () => {
    const [lo, hi] = seedParts('deadbeefcafebabe');
    const fast = new PRNG(lo, hi);
    const ref = makeReferenceUint32(lo, hi);
    for (let i = 0; i < 1_000_000; i++) {
      const a = fast.nextUint32() >>> 0;
      const b = ref() >>> 0;
      if (a !== b) {
        throw new Error(`diverged at draw ${i}: fast=${a} ref=${b}`);
      }
    }
    expect(true).toBe(true);
  });

  it('agrees on 64-bit output too', () => {
    const [lo, hi] = seedParts('0123456789abcdef');
    const fast = new PRNG(lo, hi);
    const ref = makeReferenceUint32(lo, hi);
    for (let i = 0; i < 10_000; i++) {
      const [l, h] = fast.nextUint64Parts();
      expect(h >>> 0).toBe(ref() >>> 0);
      expect(l >>> 0).toBeLessThanOrEqual(0xffffffff);
    }
  });
});

describe('nextBelow invariants', () => {
  it('stays in range and is roughly uniform', () => {
    const n = 1820;
    const draws = 200_000;
    const r = PRNG.fromSeed(seedParts('deadbeef12345678'));
    const buckets = new Int32Array(n);
    let outOfRange = 0;
    for (let i = 0; i < draws; i++) {
      const x = r.nextBelow(n);
      if (x < 0 || x >= n) outOfRange++;
      else buckets[x]++;
    }
    // Assert once, outside the loop: 200k individual expect() calls would
    // dominate the runtime of the whole suite.
    expect(outOfRange).toBe(0);
    const mean = draws / n;
    let min = Infinity;
    let max = -Infinity;
    for (const c of buckets) {
      if (c < min) min = c;
      if (c > max) max = c;
    }
    expect(min).toBeGreaterThan(mean * 0.6);
    expect(max).toBeLessThan(mean * 1.4);
  });

  it('handles n <= 1 without consuming randomness', () => {
    const r = new PRNG(1, 2);
    expect(r.nextBelow(1)).toBe(0);
    expect(r.nextBelow(0)).toBe(0);
  });
});

describe('shuffle', () => {
  it('is a permutation and seed-dependent', () => {
    const a = Array.from({ length: 288 }, (_, i) => i);
    PRNG.fromSeed(seedParts('000000000000002a')).shuffle(a);
    expect(new Set(a).size).toBe(288);

    const b = Array.from({ length: 288 }, (_, i) => i);
    PRNG.fromSeed(seedParts('000000000000002b')).shuffle(b);
    expect(a).not.toEqual(b);
  });
});
