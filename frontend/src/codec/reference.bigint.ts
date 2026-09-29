/**
 * splitmix64 written with BigInt — the readability oracle for the 32-bit
 * chunked implementation in prng.ts.
 *
 * It is ~10-30x slower per draw, so it is never used in the encoder. Its only
 * job is to let a test assert that the fast version agrees over 10^6
 * consecutive draws, which kills the entire class of limb-arithmetic bugs that
 * the chunked version is otherwise exposed to.
 */

const GAMMA = 0x9e3779b97f4a7c15n;
const M1 = 0xbf58476d1ce4e5b9n;
const M2 = 0x94d049bb133111ebn;
const MASK = (1n << 64n) - 1n;

export function makeReferenceSplitmix64(seedLo: number, seedHi: number): () => bigint {
  let state = ((BigInt(seedHi >>> 0) << 32n) | BigInt(seedLo >>> 0)) & MASK;
  return () => {
    state = (state + GAMMA) & MASK;
    let z = state;
    z = ((z ^ (z >> 30n)) * M1) & MASK;
    z = ((z ^ (z >> 27n)) * M2) & MASK;
    return (z ^ (z >> 31n)) & MASK;
  };
}

/** The reference's 32-bit output: the high half, matching prng.ts. */
export function makeReferenceUint32(seedLo: number, seedHi: number): () => number {
  const next = makeReferenceSplitmix64(seedLo, seedHi);
  return () => Number(next() >> 32n);
}
