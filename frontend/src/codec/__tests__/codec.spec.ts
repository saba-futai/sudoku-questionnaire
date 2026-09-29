import { describe, expect, it } from 'vitest';

import { layout, LayoutId } from '../layout';
import { buildTable, Table } from '../table';
import { Validity } from '../validity';
import { PRNG } from '../prng';
import { decode, encode, inspect, CodecError } from '../classic';
import { sha256, toHex } from '../sha256';
import { bytesToHex, hexToBytes, seedParts, validityArtifact, vector } from './vectors';

interface LayoutsVector {
  layouts: Record<string, { hint: string[]; padding: string[] }>;
}

interface TableVector {
  cases: {
    key: string;
    layout: string;
    fingerprint: string;
    decodeMapSize: number;
    counts: number[];
    layoutSha256: string;
    hintAlphabet: string[];
  }[];
}

interface E2EVector {
  cases: {
    name: string;
    key: string;
    layout: string;
    maxPad: number;
    seedHex: string;
    plaintextHex: string;
    wireHex: string;
    wireSha256: string;
  }[];
}

const layoutsV = vector<LayoutsVector>('layouts');
const tableV = vector<TableVector>('table');
const e2eV = vector<E2EVector>('e2e');

const validity = new Validity(validityArtifact());

const tableCache = new Map<string, Table>();
function tableFor(key: string, layoutId: LayoutId): Table {
  const ck = `${key}\u0000${layoutId}`;
  let t = tableCache.get(ck);
  if (!t) {
    t = buildTable(key, layoutId, validity);
    tableCache.set(ck, t);
  }
  return t;
}

describe('layouts', () => {
  for (const [id, want] of Object.entries(layoutsV.layouts)) {
    describe(id, () => {
      const l = layout(id as LayoutId);

      it('maps groups to the pinned clue bytes', () => {
        const got = Array.from({ length: 64 }, (_, g) => l.hintByte(g).toString(16).padStart(2, '0'));
        expect(got).toEqual(want.hint);
      });

      it('uses the pinned padding pool', () => {
        expect(Array.from(l.padding, (b) => b.toString(16).padStart(2, '0'))).toEqual(want.padding);
      });

      it('keeps padding disjoint from the clue alphabet', () => {
        l.validate();
        for (const p of l.padding) expect(l.groupOf(p)).toBe(-1);
      });

      it('round-trips every group through its wire byte', () => {
        for (let g = 0; g < 64; g++) expect(l.groupOf(l.hintByte(g))).toBe(g);
      });
    });
  }

  it('rejects bytes outside the alphabet instead of swallowing them', () => {
    // Upstream's `(b & 0x40) == 0x40` would accept all of these.
    const l = layout('ascii');
    for (const b of [0x00, 0x19, 0x7f, 0x80, 0xa9, 0xc0, 0xe4, 0xff]) {
      expect(l.groupOf(b)).toBe(-1);
      expect(l.isPadding(b)).toBe(false);
    }
  });
});

describe('table derivation', () => {
  for (const c of tableV.cases) {
    it(`${JSON.stringify(c.key)} / ${c.layout} derives the pinned table`, () => {
      const t = tableFor(c.key, c.layout as LayoutId);
      expect(t.fingerprint).toBe(c.fingerprint);
      expect(t.decodeMapSize()).toBe(c.decodeMapSize);
      expect(Array.from({ length: 256 }, (_, b) => t.encodingCount(b))).toEqual(c.counts);
      expect(t.layout.id).toBe(c.layout);
    });

    it(`${JSON.stringify(c.key)} / ${c.layout} has the pinned alphabet`, () => {
      const l = layout(c.layout as LayoutId);
      const alphabet = [...Array.from({ length: 64 }, (_, g) => l.hintByte(g)), ...l.padding];
      expect(toHex(sha256(Uint8Array.from(alphabet)))).toBe(c.layoutSha256);
      expect(Array.from({ length: 64 }, (_, g) => l.hintByte(g).toString(16).padStart(2, '0'))).toEqual(
        c.hintAlphabet,
      );
    });
  }

  it('gives every byte at least one encoding', () => {
    const t = tableFor('test-key-1', 'ascii');
    for (let b = 0; b < 256; b++) expect(t.encodingCount(b)).toBeGreaterThan(0);
  });

  it('fingerprints the key derivation, not the layout', () => {
    expect(tableFor('test-key-1', 'ascii').fingerprint).toBe(tableFor('test-key-1', 'entropy').fingerprint);
    expect(tableFor('test-key-1', 'ascii').fingerprint).not.toBe(tableFor('', 'ascii').fingerprint);
  });
});

describe('end-to-end vectors (byte-identical to Go)', () => {
  for (const c of e2eV.cases) {
    it(`${c.name}: encodes to the exact bytes Go produced`, () => {
      const t = tableFor(c.key, c.layout as LayoutId);
      const plain = hexToBytes(c.plaintextHex);
      const wire = encode(t, PRNG.fromSeed(seedParts(c.seedHex)), plain, c.maxPad);
      expect(bytesToHex(wire)).toBe(c.wireHex);
      expect(toHex(sha256(wire))).toBe(c.wireSha256);
    });

    it(`${c.name}: decodes Go's bytes back to the plaintext`, () => {
      const t = tableFor(c.key, c.layout as LayoutId);
      const plain = hexToBytes(c.plaintextHex);
      expect(bytesToHex(decode(t, hexToBytes(c.wireHex)))).toBe(c.plaintextHex);
      // and its own output
      const wire = encode(t, PRNG.fromSeed(seedParts(c.seedHex)), plain, c.maxPad);
      expect(bytesToHex(decode(t, wire))).toBe(c.plaintextHex);
    });
  }
});

describe('frame behaviour', () => {
  const t = tableFor('frame', 'ascii');

  it('is exactly 4N+4 bytes without padding', () => {
    for (const n of [0, 1, 7, 512]) {
      const wire = encode(t, PRNG.random(), new Uint8Array(n), 0);
      expect(wire.length).toBe(n * 4 + 4);
    }
  });

  it('varies ciphertext for identical plaintext', () => {
    const plain = Uint8Array.from([121, 101, 115]); // "yes"
    const forms = new Set<string>();
    for (let i = 0; i < 64; i++) forms.add(bytesToHex(encode(t, PRNG.random(), plain, 0)));
    // Identical answers must not be linkable across respondents.
    expect(forms.size).toBeGreaterThan(40);
  });

  it('rejects a byte outside the alphabet', () => {
    const wire = encode(t, PRNG.random(), Uint8Array.from([1, 2, 3]), 0);
    const bad = Uint8Array.from(wire);
    bad[2] = 0xff;
    expect(() => decode(t, bad)).toThrow(CodecError);
    try {
      decode(t, bad);
    } catch (e) {
      expect((e as CodecError).kind).toBe('invalid-wire-byte');
    }
  });

  // Targets the checksum TRAILER specifically. Replacing a body clue byte is
  // deliberately not tested: it merely selects a different valid puzzle for the
  // same plaintext byte, which is a legitimate alternative ciphertext.
  it('detects a corrupted checksum trailer', () => {
    const l = layout('ascii');
    const plain = new TextEncoder().encode('immutable record');
    for (let delta = 1; delta <= 4; delta++) {
      const wire = encode(t, PRNG.fromSeed(seedParts(delta.toString(16))), plain, 0);
      const hints: number[] = [];
      for (const b of wire) {
        const g = l.groupOf(b);
        if (g >= 0) hints.push(g);
      }
      const n = hints.length / 4 - 1;
      let want = 0;
      for (let i = 0; i < 4; i++) want = ((want << 6) | (hints[n * 4 + i] & 0x3f)) >>> 0;
      const alt = (want + delta) & 0xffffff;

      const bad = Uint8Array.from(wire);
      for (let i = 0; i < 4; i++) bad[n * 4 + i] = l.hintByte((alt >>> (18 - 6 * i)) & 0x3f);

      let caught: unknown;
      try {
        decode(t, bad);
      } catch (e) {
        caught = e;
      }
      expect(caught).toBeInstanceOf(CodecError);
      expect((caught as CodecError).kind).toBe('checksum-mismatch');
    }
  });

  it('detects a removed clue symbol', () => {
    const wire = encode(t, PRNG.random(), new TextEncoder().encode('drop a clue'), 0);
    const bad = Uint8Array.from([...wire.subarray(0, 5), ...wire.subarray(6)]);
    let caught: unknown;
    try {
      decode(t, bad);
    } catch (e) {
      caught = e;
    }
    expect(caught).toBeInstanceOf(CodecError);
    expect((caught as CodecError).kind).toBe('bad-frame');
  });

  it('rejects truncation', () => {
    const wire = encode(t, PRNG.random(), new TextEncoder().encode('truncate me please'), 0);
    for (const cut of [0, 1, 4, wire.length >> 1, wire.length - 1]) {
      expect(() => decode(t, wire.subarray(0, cut))).toThrow(CodecError);
    }
  });

  it('rejects the wrong key and the wrong layout', () => {
    const wire = encode(tableFor('right', 'ascii'), PRNG.random(), new TextEncoder().encode('x'), 0);
    expect(() => decode(tableFor('wrong', 'ascii'), wire)).toThrow(CodecError);
    expect(() => decode(tableFor('right', 'entropy'), wire)).toThrow(CodecError);
  });

  it('treats padding as transparent and reports it', () => {
    const plain = new TextEncoder().encode('padded payload');
    for (const maxPad of [0, 1, 3, 8]) {
      const wire = encode(t, PRNG.random(), plain, maxPad);
      expect(bytesToHex(decode(t, wire))).toBe(bytesToHex(plain));
    }
    const padded = encode(t, PRNG.random(), plain, 8);
    const rep = inspect(t, padded);
    expect(rep.ok).toBe(true);
    expect(rep.paddingBytes).toBeGreaterThan(0);
    expect(rep.clueSymbols).toBe(plain.length * 4 + 4);
  });

  it('explains malformed input instead of only failing', () => {
    const wire = encode(t, PRNG.random(), new TextEncoder().encode('report me'), 0);
    const bad = Uint8Array.from([...wire, 0x01]);
    const rep = inspect(t, bad);
    expect(rep.rejectedByte).toBe(0x01);
  });

  it('keeps ASCII ciphertext inside the layout alphabet', () => {
    const l = layout('ascii');
    const plain = new Uint8Array(1024);
    for (let i = 0; i < plain.length; i++) plain[i] = (i * 7) & 0xff;
    const wire = encode(t, PRNG.random(), plain, 4);
    for (const b of wire) {
      expect(l.groupOf(b) >= 0 || l.isPadding(b)).toBe(true);
      expect(b).not.toBe(0);
    }
  });

  it('handles empty plaintext', () => {
    const wire = encode(t, PRNG.random(), new Uint8Array(0), 0);
    expect(wire.length).toBe(4);
    expect(decode(t, wire).length).toBe(0);
  });
});
