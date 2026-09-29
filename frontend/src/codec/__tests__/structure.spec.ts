import { describe, expect, it } from 'vitest';

import { grids, gridsSha256, GRID_COUNT } from '../grids';
import { posSets, posSetsSha256, POS_SET_COUNT } from '../possets';
import { computeValidity, VALIDITY_BYTES } from '../validity';
import { toHex, sha256 } from '../sha256';
import { hexToBytes, validityArtifact, vector } from './vectors';

interface StructureVector {
  gridCount: number;
  gridsSha256: string;
  gridsFirstHex: string;
  gridsLastHex: string;
  posSetCount: number;
  posSetsSha256: string;
  posSetsFirstHex: string;
  posSetsLastHex: string;
}

interface ValidityVector {
  sha256: string;
  bytes: number;
  perGridCounts: number[];
  histogram: Record<string, number>;
  grid0Positions: number[];
}

const s = vector<StructureVector>('structure');
const vv = vector<ValidityVector>('validity');

describe('grid enumeration', () => {
  it('produces 288 grids in the pinned order', () => {
    expect(GRID_COUNT).toBe(s.gridCount);
    expect(grids()).toHaveLength(s.gridCount);
    expect(gridsSha256()).toBe(s.gridsSha256);
    expect(Array.from(grids()[0])).toEqual(Array.from(hexToBytes(s.gridsFirstHex)));
    expect(Array.from(grids()[287])).toEqual(Array.from(hexToBytes(s.gridsLastHex)));
  });

  it('every grid is a legal sudoku', () => {
    for (const g of grids()) {
      for (let i = 0; i < 16; i++) {
        expect(g[i]).toBeGreaterThanOrEqual(1);
        expect(g[i]).toBeLessThanOrEqual(4);
      }
      for (let k = 0; k < 4; k++) {
        const row = new Set<number>();
        const col = new Set<number>();
        const box = new Set<number>();
        for (let j = 0; j < 4; j++) {
          row.add(g[k * 4 + j]);
          col.add(g[j * 4 + k]);
          const br = ((k / 2) | 0) * 2;
          const bc = (k % 2) * 2;
          box.add(g[(br + ((j / 2) | 0)) * 4 + bc + (j % 2)]);
        }
        expect(row.size).toBe(4);
        expect(col.size).toBe(4);
        expect(box.size).toBe(4);
      }
    }
  });
});

describe('position sets', () => {
  it('produces 1820 ascending quadruples in lexicographic order', () => {
    expect(POS_SET_COUNT).toBe(s.posSetCount);
    expect(posSets()).toHaveLength(s.posSetCount);
    expect(posSetsSha256()).toBe(s.posSetsSha256);
    expect(Array.from(posSets()[0])).toEqual(Array.from(hexToBytes(s.posSetsFirstHex)));
    expect(Array.from(posSets()[1819])).toEqual(Array.from(hexToBytes(s.posSetsLastHex)));
    for (const p of posSets()) {
      expect(p[0]).toBeLessThan(p[1]);
      expect(p[1]).toBeLessThan(p[2]);
      expect(p[2]).toBeLessThan(p[3]);
    }
  });
});

describe('validity bitmap', () => {
  it('the shipped artifact has the pinned digest', () => {
    const art = validityArtifact();
    expect(art.length).toBe(VALIDITY_BYTES);
    expect(art.length).toBe(vv.bytes);
    expect(toHex(sha256(art))).toBe(vv.sha256);
  });

  it('computing it from scratch reproduces the shipped artifact byte for byte', () => {
    // This is what keeps the browser a complete implementation of the spec
    // rather than a consumer of a server artifact.
    const fresh = computeValidity();
    const art = validityArtifact();
    expect(toHex(sha256(fresh.bits))).toBe(vv.sha256);
    expect(fresh.bits).toEqual(art);
  }, 120_000);

  it('has the two-orbit structure (96 grids x 12, 192 grids x 128)', () => {
    const hist: Record<string, number> = {};
    for (const c of computeValidity().counts()) {
      hist[String(c)] = (hist[String(c)] ?? 0) + 1;
    }
    expect(hist).toEqual(vv.histogram);
    expect(vv.perGridCounts).toHaveLength(288);
  }, 120_000);

  it('marks the same position sets valid for grid 0', () => {
    const art = validityArtifact();
    const found: number[] = [];
    for (let p = 0; p < POS_SET_COUNT; p++) {
      if ((art[(p >> 3)] & (1 << (p & 7))) !== 0) found.push(p);
    }
    expect(found).toEqual(vv.grid0Positions);
  });
});
