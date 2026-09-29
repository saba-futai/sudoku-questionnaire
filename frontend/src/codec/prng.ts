/**
 * splitmix64, mirrored bit-for-bit from backend/internal/codec/prng.go.
 *
 * The 64-bit state is held as two 32-bit halves. BigInt would be far easier to
 * read but allocates and runs ~10-30x slower per draw; since the encoder draws
 * once or twice per plaintext byte, that difference is measurable on a
 * multi-hundred-KB image payload. `reference.bigint.ts` implements the same
 * generator with BigInt and a test asserts the two agree over 10^6 consecutive
 * draws, which is what makes the fast version trustworthy.
 *
 * Every hazard called out in spec/CODEC.md §5.3 is handled here explicitly:
 * logical shifts only (`>>>`), `>>> 0` after every operation, `Math.imul` for
 * all 32-bit multiplies, and a 64-bit carry handled in doubles where it is
 * exact.
 */

import { sha256Utf8 } from './sha256';

/** Domain separator; must equal codec.SeedDomain in Go. */
export const SEED_DOMAIN = 'QRforQA:sudoku-codec:v1';

const GAMMA_LO = 0x7f4a7c15;
const GAMMA_HI = 0x9e3779b9;
const M1_LO = 0x1ce4e5b9;
const M1_HI = 0xbf58476d;
const M2_LO = 0x133111eb;
const M2_HI = 0x94d049bb;

/**
 * High 32 bits of the unsigned 32x32 product, via 16-bit limbs.
 *
 * `Math.imul` returns a SIGNED int32, so the low limbs are masked and the
 * accumulated carries shifted with `>>>` rather than `/`.
 */
function umulh(u: number, v: number): number {
  const u0 = u & 0xffff;
  const u1 = u >>> 16;
  const v0 = v & 0xffff;
  const v1 = v >>> 16;
  const p00 = Math.imul(u0, v0);
  const p01 = Math.imul(u0, v1);
  const p10 = Math.imul(u1, v0);
  const p11 = Math.imul(u1, v1);
  const mid = (p00 >>> 16) + (p01 & 0xffff) + (p10 & 0xffff);
  return (p11 + (p01 >>> 16) + (p10 >>> 16) + (mid >>> 16)) >>> 0;
}

/** Derives the per-key table seed, returning the low and high 32-bit halves. */
export function tableSeedLoHi(key: string): [number, number] {
  // Byte sequence matches Go's successive h.Write calls: domain, 0x00, key.
  const h = sha256Utf8(`${SEED_DOMAIN}\u0000${key}`);
  const hi = ((h[0] << 24) | (h[1] << 16) | (h[2] << 8) | h[3]) >>> 0;
  const lo = ((h[4] << 24) | (h[5] << 16) | (h[6] << 8) | h[7]) >>> 0;
  return [lo, hi];
}

export class PRNG {
  private s0: number;
  private s1: number;
  private outLo = 0;
  private outHi = 0;

  constructor(seedLo: number, seedHi: number) {
    this.s0 = seedLo >>> 0;
    this.s1 = seedHi >>> 0;
  }

  static fromSeed([lo, hi]: [number, number]): PRNG {
    return new PRNG(lo, hi);
  }

  /** Seeds from the OS CSPRNG. Used for the encoder's obfuscation stream, which
   * deliberately need not be reproducible across languages. */
  static random(): PRNG {
    const b = new Uint32Array(2);
    crypto.getRandomValues(b);
    return new PRNG(b[0], b[1]);
  }

  /** Advances the state once, leaving the output in outLo/outHi. */
  private step(): void {
    // --- state += GAMMA, with an explicit carry -------------------------
    // s0 + GAMMA_LO <= 2^33, which is exactly representable in a double.
    const t = this.s0 + GAMMA_LO;
    const lo = t >>> 0;
    const carry = t > 0xffffffff ? 1 : 0;
    const hi = (this.s1 + GAMMA_HI + carry) >>> 0;
    this.s0 = lo;
    this.s1 = hi;

    let z0 = lo;
    let z1 = hi;

    // --- z ^= z >>> 30 ---------------------------------------------------
    z0 = (z0 ^ ((z0 >>> 30) | (z1 << 2))) >>> 0;
    z1 = (z1 ^ (z1 >>> 30)) >>> 0;

    // --- z *= M1 (mod 2^64) ---------------------------------------------
    {
      const a0 = Math.imul(z0, M1_LO) >>> 0;
      const a1 = (umulh(z0, M1_LO) + Math.imul(z0, M1_HI) + Math.imul(z1, M1_LO)) >>> 0;
      z0 = a0;
      z1 = a1;
    }

    // --- z ^= z >>> 27 ---------------------------------------------------
    z0 = (z0 ^ ((z0 >>> 27) | (z1 << 5))) >>> 0;
    z1 = (z1 ^ (z1 >>> 27)) >>> 0;

    // --- z *= M2 (mod 2^64) ---------------------------------------------
    {
      const a0 = Math.imul(z0, M2_LO) >>> 0;
      const a1 = (umulh(z0, M2_LO) + Math.imul(z0, M2_HI) + Math.imul(z1, M2_LO)) >>> 0;
      z0 = a0;
      z1 = a1;
    }

    // --- z ^= z >>> 31 ---------------------------------------------------
    z0 = (z0 ^ ((z0 >>> 31) | (z1 << 1))) >>> 0;
    z1 = (z1 ^ (z1 >>> 31)) >>> 0;

    this.outLo = z0;
    this.outHi = z1;
  }

  /** Next 32-bit output: the HIGH half of the 64-bit result, as in Go. */
  nextUint32(): number {
    this.step();
    return this.outHi;
  }

  /** Full 64-bit output, for test-vector comparison. */
  nextUint64Parts(): [number, number] {
    this.step();
    return [this.outLo, this.outHi];
  }

  nextUint64(): bigint {
    const [lo, hi] = this.nextUint64Parts();
    return (BigInt(hi) << 32n) | BigInt(lo);
  }

  /**
   * Uniform value in [0, n) by rejection sampling.
   *
   * `limit` must be computed as a double: in Go, `(1<<32)/n*n` evaluated with
   * uint32 arithmetic silently yields 0 and loops forever — the mirror image of
   * that bug is losing precision here, so 2^32 is written literally.
   */
  nextBelow(n: number): number {
    if (n <= 1) return 0;
    const limit = Math.floor(0x100000000 / n) * n;
    for (;;) {
      const x = this.nextUint32();
      if (x < limit) return x % n;
    }
  }

  /** In-place Fisher-Yates, descending. Direction is pinned. */
  shuffle<T>(a: T[]): void {
    for (let i = a.length - 1; i > 0; i--) {
      const j = this.nextBelow(i + 1);
      const t = a[i];
      a[i] = a[j];
      a[j] = t;
    }
  }

  pick<T>(a: T[] | readonly T[]): T {
    return a[this.nextBelow(a.length)];
  }
}
