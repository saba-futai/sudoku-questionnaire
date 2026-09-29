package codec

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
)

// Grid is a 4x4 sudoku laid out row-major: index = row*4 + col, values 1..4.
// The 2x2 boxes are at (row/2)*2, (col/2)*2.
type Grid [16]uint8

// GridCount is the number of valid 4x4 sudoku grids. It exceeds 256, which is
// precisely why a byte can be mapped to a grid injectively.
const GridCount = 288

// generateGrids enumerates every valid 4x4 sudoku by backtracking.
//
// The enumeration ORDER is load-bearing, not incidental: the per-key shuffle
// permutes indices into this slice, so a different traversal yields 288
// "correct" grids in a different order and therefore a completely different
// table. spec/vectors/grids.json pins the result by hash.
//
// Traversal (mirrors upstream SUDOKU-ASCII/sudoku pkg/obfs/sudoku/grid.go):
// cells in row-major order 0..15, candidate values 1..4 ascending, row/column
// conflict checked before the box conflict, resetting to 0 on backtrack.
func generateGrids() []Grid {
	grids := make([]Grid, 0, GridCount)
	var g Grid

	var backtrack func(idx int)
	backtrack = func(idx int) {
		if idx == 16 {
			grids = append(grids, g)
			return
		}
		row, col := idx/4, idx%4
		br, bc := (row/2)*2, (col/2)*2
		for num := uint8(1); num <= 4; num++ {
			valid := true
			for i := 0; i < 4; i++ {
				if g[row*4+i] == num || g[i*4+col] == num {
					valid = false
					break
				}
			}
			if valid {
				for r := 0; r < 2; r++ {
					for c := 0; c < 2; c++ {
						if g[(br+r)*4+(bc+c)] == num {
							valid = false
							break
						}
					}
				}
			}
			if valid {
				g[idx] = num
				backtrack(idx + 1)
				g[idx] = 0
			}
		}
	}
	backtrack(0)
	return grids
}

var (
	gridsOnce sync.Once
	gridsVal  []Grid
	gridsHash string
)

// Grids returns the canonical 288-grid enumeration (computed once per process).
func Grids() []Grid {
	gridsOnce.Do(func() {
		gridsVal = generateGrids()
		h := sha256.New()
		for _, g := range gridsVal {
			h.Write(g[:])
		}
		gridsHash = hex.EncodeToString(h.Sum(nil))
	})
	return gridsVal
}

// GridsSHA256 is the hex digest over the 288 grids in enumeration order. It is
// pinned in spec/vectors/grids.json and asserted on startup: if a future edit
// changes the traversal, the server refuses to start rather than silently
// deriving a table the browser cannot read.
func GridsSHA256() string {
	Grids()
	return gridsHash
}
