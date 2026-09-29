package codec

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
)

// perm4 lists all 24 permutations of {0,1,2,3} in lexicographic order.
//
// The encoder emits a puzzle's four clue bytes in a randomly chosen
// permutation. Because PackGroups sorts before lookup, decode is unaffected by
// the choice, so this permutation need not match the TypeScript side — the
// decoder never sees it.
var perm4 = func() [24][4]byte {
	var out [24][4]byte
	i := 0
	for a := 0; a < 4; a++ {
		for b := 0; b < 4; b++ {
			if b == a {
				continue
			}
			for c := 0; c < 4; c++ {
				if c == a || c == b {
					continue
				}
				for d := 0; d < 4; d++ {
					if d == a || d == b || d == c {
						continue
					}
					out[i] = [4]byte{byte(a), byte(b), byte(c), byte(d)}
					i++
				}
			}
		}
	}
	return out
}()

// Table is the keyed mapping between plaintext bytes and solvable puzzles.
//
// It is immutable once built and safe for concurrent readers.
type Table struct {
	key         string
	layout      *Layout
	encode      [256][][4]byte
	decode      map[uint32]byte
	fingerprint string
}

// BuildTable derives the per-key mapping.
//
// Per byte value b, exactly one grid is selected (shuffledGrids[b]); the byte's
// valid puzzles are that grid's uniquely-identifying clue sets, inherited
// directly from the key-independent validity bitmap.
func BuildTable(key string, layoutID LayoutID, v *Validity) (*Table, error) {
	lay := NewLayout(layoutID)
	if err := lay.ValidateLayout(); err != nil {
		return nil, err
	}
	if v == nil {
		return nil, fmt.Errorf("codec: nil validity bitmap")
	}

	t := &Table{
		key:    key,
		layout: lay,
		decode: make(map[uint32]byte, 256*90),
	}

	order := make([]int, GridCount)
	for i := range order {
		order[i] = i
	}
	rng := NewPRNG(TableSeed(key))
	rng.Shuffle(GridCount, func(i, j int) { order[i], order[j] = order[j], order[i] })

	grids := Grids()
	pos := PosSets()

	for b := 0; b < 256; b++ {
		gi := order[b]
		g := &grids[gi]
		encs := make([][4]byte, 0, 128)
		for pi, p := range pos {
			if !v.Valid(gi, pi) {
				continue
			}
			var gr [4]byte
			for i := 0; i < 4; i++ {
				gr[i] = groupOfGrid(g, p[i])
			}
			encs = append(encs, gr)
			t.decode[PackGroups(gr)] = byte(b)
		}
		if len(encs) == 0 {
			return nil, fmt.Errorf("codec: byte %d has no valid encoding under key %q", b, key)
		}
		t.encode[b] = encs
	}

	t.fingerprint = t.computeFingerprint()
	return t, nil
}

// computeFingerprint hashes the encode table canonically, so the server and the
// browser can prove they derived the same mapping without shipping a table.
func (t *Table) computeFingerprint() string {
	h := sha256.New()
	var n [2]byte
	for b := 0; b < 256; b++ {
		encs := t.encode[b]
		binary.BigEndian.PutUint16(n[:], uint16(len(encs)))
		h.Write(n[:])
		for _, e := range encs {
			h.Write(e[:])
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Fingerprint returns the hex digest identifying this (key, layout) mapping.
func (t *Table) Fingerprint() string { return t.fingerprint }

// Layout returns the table's wire layout.
func (t *Table) Layout() *Layout { return t.layout }

// Key returns the derivation key.
func (t *Table) Key() string { return t.key }

// EncodingCount returns how many distinct puzzles exist for plaintext byte b.
func (t *Table) EncodingCount(b byte) int { return len(t.encode[b]) }

// EncodeCounts returns the encoding count for all 256 byte values, for the
// codec inspector.
func (t *Table) EncodeCounts() [256]int {
	var out [256]int
	for b := 0; b < 256; b++ {
		out[b] = len(t.encode[b])
	}
	return out
}

// DecodeMapSize returns the number of distinct clue sets the decoder accepts.
func (t *Table) DecodeMapSize() int { return len(t.decode) }
