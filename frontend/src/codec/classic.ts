/**
 * Classic codec: one plaintext byte becomes one uniquely-solvable four-clue
 * puzzle. Mirrored from backend/internal/codec/classic.go.
 *
 * Wire format per byte: four clue bytes, emitted in one of 24 randomly chosen
 * permutations, each optionally preceded by filler. A fixed four-symbol trailer
 * carries a 24-bit checksum, making the frame self-describing — the plaintext
 * length is implied by the clue count.
 */

import { crc32 } from './crc32';
import { PRNG } from './prng';
import { PERM4, Table } from './table';

export const TRAILER_BYTES = 4;
export const FRAME_OVERHEAD = TRAILER_BYTES;

/** No filler by default: this app ships ordinary JSON over HTTP, so padding
 * would only inflate storage and bandwidth. Kept configurable for the codec
 * inspector. */
export const MAX_PAD_DEFAULT = 0;

export type CodecErrorKind =
  | 'invalid-wire-byte'
  | 'bad-frame'
  | 'unknown-puzzle'
  | 'checksum-mismatch';

export class CodecError extends Error {
  constructor(
    readonly kind: CodecErrorKind,
    message: string,
  ) {
    super(message);
    this.name = 'CodecError';
  }
}

/**
 * Encodes plaintext into sudoku ciphertext.
 *
 * The RNG draw order is pinned to match Go exactly, because the conformance
 * test asserts the two implementations produce byte-identical ciphertext for a
 * given seed. Decoding does not depend on it (the decoder sorts each clue set),
 * but vector equality does.
 */
export function encode(table: Table, rng: PRNG, plain: Uint8Array, maxPad = MAX_PAD_DEFAULT): Uint8Array {
  const lay = table.layout;
  const pads = lay.padding;
  const cap = plain.length * 4 * (1 + maxPad) + FRAME_OVERHEAD;
  const out = new Uint8Array(cap);
  let o = 0;

  for (let i = 0; i < plain.length; i++) {
    const b = plain[i];
    const qo = table.pickQuad(b, rng);
    const perm = PERM4[rng.nextBelow(PERM4.length)];
    for (let k = 0; k < 4; k++) {
      if (maxPad > 0) {
        const n = rng.nextBelow(maxPad + 1);
        for (let z = 0; z < n; z++) out[o++] = pads[rng.nextBelow(pads.length)];
      }
      out[o++] = lay.hintByte(table.quads[qo + perm[k]]);
    }
  }

  const c = crc32(plain) & 0xffffff;
  for (let shift = 18; shift >= 0; shift -= 6) {
    out[o++] = lay.hintByte((c >>> shift) & 0x3f);
  }

  // With no padding the reservation is exact, so the common path copies nothing.
  return o === out.length ? out : out.slice(0, o);
}

/** Recovers plaintext, throwing CodecError on any wire-grammar violation. */
export function decode(table: Table, wire: Uint8Array): Uint8Array {
  const lay = table.layout;

  const hints = new Uint8Array(wire.length);
  let hn = 0;
  for (let i = 0; i < wire.length; i++) {
    const b = wire[i];
    const g = lay.groupOf(b);
    if (g >= 0) {
      hints[hn++] = g;
      continue;
    }
    if (lay.isPadding(b)) continue;
    throw new CodecError(
      'invalid-wire-byte',
      `byte 0x${b.toString(16).padStart(2, '0')} at offset ${i} is neither a clue nor padding`,
    );
  }

  if (hn < TRAILER_BYTES || (hn - TRAILER_BYTES) % 4 !== 0) {
    throw new CodecError('bad-frame', `${hn} clue symbols`);
  }

  const n = (hn - TRAILER_BYTES) / 4;
  const out = new Uint8Array(n);
  for (let i = 0; i < n; i++) {
    const o = i * 4;
    const v = table.lookup(hints[o], hints[o + 1], hints[o + 2], hints[o + 3]);
    if (v < 0) throw new CodecError('unknown-puzzle', `puzzle ${i} maps to no plaintext byte`);
    out[i] = v;
  }

  let want = 0;
  for (let i = 0; i < TRAILER_BYTES; i++) {
    want = ((want << 6) | (hints[n * 4 + i] & 0x3f)) >>> 0;
  }
  const got = crc32(out) & 0xffffff;
  if (got !== want) {
    throw new CodecError(
      'checksum-mismatch',
      `got ${got.toString(16).padStart(6, '0')}, frame says ${want.toString(16).padStart(6, '0')}`,
    );
  }
  return out;
}

export interface DecodeReport {
  ok: boolean;
  plaintext: Uint8Array;
  clueSymbols: number;
  paddingBytes: number;
  rejectedByte: number; // -1 when the frame is grammatically clean
  trailerHex: string;
  checksumOk: boolean;
  error?: string;
}

/**
 * Explains a ciphertext without failing fast, so the codec inspector can show
 * what went wrong instead of only that something did.
 */
export function inspect(table: Table, wire: Uint8Array): DecodeReport {
  const lay = table.layout;
  const rep: DecodeReport = {
    ok: false,
    plaintext: new Uint8Array(0),
    clueSymbols: 0,
    paddingBytes: 0,
    rejectedByte: -1,
    trailerHex: '',
    checksumOk: false,
  };

  const hints = new Uint8Array(wire.length);
  let hn = 0;
  for (const b of wire) {
    const g = lay.groupOf(b);
    if (g >= 0) {
      hints[hn++] = g;
      continue;
    }
    if (lay.isPadding(b)) {
      rep.paddingBytes++;
      continue;
    }
    if (rep.rejectedByte < 0) rep.rejectedByte = b;
  }
  rep.clueSymbols = hn;

  if (hn < TRAILER_BYTES || (hn - TRAILER_BYTES) % 4 !== 0) {
    rep.error = `clue count ${hn} is not 4N+4`;
    return rep;
  }

  const n = (hn - TRAILER_BYTES) / 4;
  const out = new Uint8Array(n);
  for (let i = 0; i < n; i++) {
    const o = i * 4;
    const v = table.lookup(hints[o], hints[o + 1], hints[o + 2], hints[o + 3]);
    if (v < 0) {
      rep.error = `puzzle ${i} maps to no plaintext byte`;
      return rep;
    }
    out[i] = v;
  }

  let want = 0;
  for (let i = 0; i < TRAILER_BYTES; i++) {
    want = ((want << 6) | (hints[n * 4 + i] & 0x3f)) >>> 0;
  }
  rep.plaintext = out;
  rep.trailerHex = want.toString(16).padStart(6, '0');
  rep.checksumOk = (crc32(out) & 0xffffff) === want;
  rep.ok = rep.checksumOk && rep.rejectedByte < 0;
  return rep;
}
