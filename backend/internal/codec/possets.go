package codec

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"sync"
)

// PosSetCount is C(16,4): the number of ways to choose the 4 clue cells of a
// puzzle from a 16-cell grid.
const PosSetCount = 1820

// A PosSet is an ascending quadruple of cell indices.
type PosSet [4]byte

// buildPosSets enumerates all C(16,4) cell quadruples in lexicographic order:
// index 0 is {0,1,2,3}, index 1819 is {12,13,14,15}, with the last index
// varying fastest.
//
// This order is pinned for the same reason as the grid traversal: it defines
// the bit positions inside the validity bitmap and the identity of every
// position-set index in the test vectors.
func buildPosSets() []PosSet {
	ps := make([]PosSet, 0, PosSetCount)
	for a := 0; a < 13; a++ {
		for b := a + 1; b < 14; b++ {
			for c := b + 1; c < 15; c++ {
				for d := c + 1; d < 16; d++ {
					ps = append(ps, PosSet{byte(a), byte(b), byte(c), byte(d)})
				}
			}
		}
	}
	return ps
}

var (
	posSetsOnce sync.Once
	posSetsVal  []PosSet
	posSetsHash string
)

// PosSets returns the canonical 1820 position sets (computed once per process).
func PosSets() []PosSet {
	posSetsOnce.Do(func() {
		posSetsVal = buildPosSets()
		h := sha256.New()
		var buf [4]byte
		for _, p := range posSetsVal {
			for _, v := range p {
				buf[0], buf[1] = 0, v
				h.Write(buf[:2])
			}
		}
		posSetsHash = hex.EncodeToString(h.Sum(nil))
	})
	return posSetsVal
}

// PosSetsSHA256 is the hex digest over the 1820 position sets packed as
// big-endian uint16s, pinned in spec/vectors/possets.json.
func PosSetsSHA256() string {
	PosSets()
	return posSetsHash
}

// PackPosSets serialises the position sets for hashing/vector generation.
func PackPosSets() []byte {
	ps := PosSets()
	out := make([]byte, 0, len(ps)*8)
	var buf [2]byte
	for _, p := range ps {
		for _, v := range p {
			binary.BigEndian.PutUint16(buf[:], uint16(v))
			out = append(out, buf[:]...)
		}
	}
	return out
}
