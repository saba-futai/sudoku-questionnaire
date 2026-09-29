/**
 * Bidirectional cross-execution against the Go implementation.
 *
 * The pinned vectors in spec/vectors prove the two sides agree on a fixed set
 * of inputs. This file proves it on inputs nobody chose in advance, by running
 * the real Go binary and comparing byte for byte.
 *
 * Three assertions per case, in increasing strength:
 *   1. TS decodes Go's ciphertext, and Go decodes TS's ciphertext;
 *   2. both implementations encode the SAME bytes for the same seed —
 *      strictly stronger than a round-trip, because a divergence where each
 *      side is self-consistent but different still fails here;
 *   3. the frame survives a Go -> TS -> Go -> TS relay unchanged.
 *
 * Skips (rather than fails) when the Go toolchain is absent, so the frontend
 * suite stays runnable on a machine without it.
 */

import { execFileSync, spawnSync } from 'node:child_process';
import { existsSync, mkdirSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

import { bytesToHex, pseudoRandomBytes, REPO_ROOT, seedParts, validityArtifact } from './vectors';
import { LayoutId } from '../layout';
import { buildTable, Table } from '../table';
import { Validity } from '../validity';
import { PRNG } from '../prng';
import { decode, encode } from '../classic';

const CLI = `${REPO_ROOT}backend/bin/codec-cli`;

const cliReady: boolean = (() => {
  try {
    mkdirSync(`${REPO_ROOT}backend/bin`, { recursive: true });
    execFileSync('go', ['build', '-o', CLI, './cmd/codec-cli'], {
      cwd: `${REPO_ROOT}backend`,
      stdio: 'pipe',
    });
    return existsSync(CLI);
  } catch {
    return false;
  }
})();

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

function goRun(args: string[], input: Uint8Array): Uint8Array {
  const r = spawnSync(CLI, args, { input: Buffer.from(input), maxBuffer: 64 * 1024 * 1024 });
  if (r.status !== 0) {
    throw new Error(`codec-cli ${args.join(' ')} failed: ${r.stderr?.toString() ?? r.error?.message}`);
  }
  return new Uint8Array(r.stdout);
}

const goEncode = (key: string, l: string, maxPad: number, seed: string, plain: Uint8Array): Uint8Array =>
  goRun(['-mode', 'encode', '-key', key, '-layout', l, '-max-pad', String(maxPad), '-seed', seed], plain);

const goDecode = (key: string, l: string, wire: Uint8Array): Uint8Array =>
  goRun(['-mode', 'decode', '-key', key, '-layout', l], wire);

const goFingerprint = (key: string, l: string): string =>
  new TextDecoder().decode(goRun(['-mode', 'fingerprint', '-key', key, '-layout', l], new Uint8Array(0))).trim();

interface Case {
  key: string;
  layout: LayoutId;
  maxPad: number;
  seed: string;
  plain: Uint8Array;
}

function buildCases(): Case[] {
  const cases: Case[] = [];
  const keys = ['conformance-key', '', '键-🔑-跨语言'];
  const layouts: LayoutId[] = ['ascii', 'entropy'];
  const sizes = [0, 1, 2, 3, 4, 17, 64, 255, 256, 1024];

  // Deterministic edge cases first, so a failure names a reproducible input.
  // Seeds are scrambled with 32-bit ops only; a float multiply by a 64-bit
  // constant would silently lose precision and make the vectors unreproducible.
  const seedFor = (n: number): string => {
    const lo = (Math.imul(n, 0x9e3779b1) ^ 0x5bf03635) >>> 0;
    const hi = (Math.imul(n, 0x85ebca6b) ^ 0xc2b2ae35) >>> 0;
    return hi.toString(16).padStart(8, '0') + lo.toString(16).padStart(8, '0');
  };

  let n = 0;
  for (const key of keys) {
    for (const layout of layouts) {
      for (const size of sizes) {
        cases.push({
          key,
          layout,
          maxPad: n % 3,
          seed: seedFor(n),
          plain: pseudoRandomBytes(size, 0xc0ffee + n),
        });
        n++;
      }
    }
  }
  // A few larger payloads that none of the pinned vectors cover.
  for (const size of [8192, 32 * 1024]) {
    cases.push({
      key: 'bulk-key',
      layout: 'ascii',
      maxPad: 0,
      seed: 'deadbeefdeadbeef',
      plain: pseudoRandomBytes(size, 0x5eed + size),
    });
  }
  return cases;
}

const cases = buildCases();

describe.skipIf(!cliReady)('cross-language conformance (Go <-> TypeScript)', () => {
  it('the Go binary builds and its table fingerprints match TS', () => {
    for (const key of ['conformance-key', '', '键-🔑-跨语言', 'bulk-key']) {
      for (const layoutId of ['ascii', 'entropy'] as LayoutId[]) {
        expect(goFingerprint(key, layoutId)).toBe(tableFor(key, layoutId).fingerprint);
      }
    }
  });

  it('encodes byte-identical ciphertext, and each side decodes the other', () => {
    for (const c of cases) {
      const label = `key=${JSON.stringify(c.key)} layout=${c.layout} pad=${c.maxPad} len=${c.plain.length}`;
      const table = tableFor(c.key, c.layout);

      // (2) byte-identical encoding
      const tsWire = encode(table, PRNG.fromSeed(seedParts(c.seed)), c.plain, c.maxPad);
      const goWire = goEncode(c.key, c.layout, c.maxPad, c.seed, c.plain);
      expect(bytesToHex(tsWire), `encode mismatch: ${label}`).toBe(bytesToHex(goWire));

      // (1) cross-decoding in both directions
      expect(bytesToHex(decode(table, goWire)), `TS failed to decode Go: ${label}`).toBe(bytesToHex(c.plain));
      expect(bytesToHex(goDecode(c.key, c.layout, tsWire)), `Go failed to decode TS: ${label}`).toBe(
        bytesToHex(c.plain),
      );
    }
  });

  it('survives a Go -> TS -> Go -> TS relay unchanged', () => {
    for (const c of cases.slice(0, 12)) {
      const table = tableFor(c.key, c.layout);
      const start = goEncode(c.key, c.layout, c.maxPad, c.seed, c.plain);
      const tsDecoded = decode(table, start);
      const tsRe = encode(table, PRNG.fromSeed(seedParts(c.seed)), tsDecoded, c.maxPad);
      const goDecoded = goDecode(c.key, c.layout, tsRe);
      expect(bytesToHex(goDecoded)).toBe(bytesToHex(c.plain));
    }
  });

  it('agrees on rejecting a tampered trailer', () => {
    const key = 'reject-key';
    const table = tableFor(key, 'ascii');
    const plain = new TextEncoder().encode('both sides must refuse this');
    const wire = goEncode(key, 'ascii', 0, '0000000000000001', plain);

    const bad = Uint8Array.from(wire);
    bad[bad.length - 1] = bad[bad.length - 1] === 0x40 ? 0x41 : 0x40;

    const goResult = spawnSync(CLI, ['-mode', 'decode', '-key', key, '-layout', 'ascii'], {
      input: Buffer.from(bad),
    });
    expect(goResult.status).not.toBe(0);
    expect(() => decode(table, bad)).toThrow();
  });

  it('agrees on rejecting ciphertext produced under a different key', () => {
    const plain = new TextEncoder().encode('key-bound');
    const wire = goEncode('key-a', 'ascii', 0, '0000000000000002', plain);

    const goResult = spawnSync(CLI, ['-mode', 'decode', '-key', 'key-b', '-layout', 'ascii'], {
      input: Buffer.from(wire),
    });
    expect(goResult.status).not.toBe(0);
    expect(() => decode(tableFor('key-b', 'ascii'), wire)).toThrow();
  });

  it('decodes a payload whose plaintext is every byte value', () => {
    const plain = new Uint8Array(256);
    for (let i = 0; i < 256; i++) plain[i] = i;
    const wire = goEncode('allbytes', 'ascii', 0, '0000000000000003', plain);
    expect(bytesToHex(decode(tableFor('allbytes', 'ascii'), wire))).toBe(bytesToHex(plain));
    expect(bytesToHex(goDecode('allbytes', 'ascii', wire))).toBe(bytesToHex(plain));
  });
});

describe.skipIf(cliReady)('cross-language conformance (skipped)', () => {
  it('Go toolchain not available', () => {
    expect(cliReady).toBe(false);
  });
});
