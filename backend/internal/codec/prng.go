package codec

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
)

// SeedDomain domain-separates the codec's key derivation from any other use of
// the same key material. See spec/CODEC.md §1.2.
const SeedDomain = "QRforQA:sudoku-codec:v1"

// splitmix64 constants.
const (
	gamma = 0x9E3779B97F4A7C15
	mix1  = 0xBF58476D1CE4E5B9
	mix2  = 0x94D049BB133111EB
)

// PRNG is the codec's deterministic generator (splitmix64).
//
// Every operation is add/xor/logical-shift/multiply-mod-2^64, so the TypeScript
// mirror in frontend/src/codec/prng.ts can reproduce it bit-for-bit. That
// bit-for-bit agreement is what lets the browser encode and the server decode.
// See spec/CODEC.md §1.3.
type PRNG struct {
	state uint64
}

// NewPRNG returns a generator positioned at seed.
func NewPRNG(seed uint64) *PRNG { return &PRNG{state: seed} }

// NewRandomPRNG seeds from the OS CSPRNG. Used for the encoder's obfuscation
// stream, which deliberately does NOT need to be reproducible across languages.
func NewRandomPRNG() *PRNG {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand failing is not recoverable in any meaningful way, and
		// reusing a fixed seed would make identical answers produce identical
		// ciphertext. Panic rather than degrade silently.
		panic("codec: crypto/rand unavailable: " + err.Error())
	}
	return NewPRNG(binary.BigEndian.Uint64(b[:]))
}

// TableSeed derives the per-key table seed. Both sides must agree on this.
func TableSeed(key string) uint64 {
	h := sha256.New()
	h.Write([]byte(SeedDomain))
	h.Write([]byte{0x00})
	h.Write([]byte(key))
	sum := h.Sum(nil)
	return binary.BigEndian.Uint64(sum[:8])
}

// NextUint64 returns the next 64-bit output.
func (r *PRNG) NextUint64() uint64 {
	r.state += gamma
	z := r.state
	z = (z ^ (z >> 30)) * mix1
	z = (z ^ (z >> 27)) * mix2
	return z ^ (z >> 31)
}

// NextUint32 returns the *high* 32 bits of the next output. Which half is used
// is arbitrary but it is pinned, because a mirror that took the low half would
// silently derive a different table.
func (r *PRNG) NextUint32() uint32 { return uint32(r.NextUint64() >> 32) }

// NextBelow returns a uniform value in [0, n) using rejection sampling.
//
// The limit must be computed in 64-bit: `(1<<32)/n*n` evaluated as uint32
// overflows to 0, which makes `x < limit` permanently false and hangs the
// caller. See spec/CODEC.md §1.3.
func (r *PRNG) NextBelow(n uint32) uint32 {
	if n <= 1 {
		return 0
	}
	limit := (uint64(1) << 32) / uint64(n) * uint64(n)
	for {
		x := r.NextUint32()
		if uint64(x) < limit {
			return x % n
		}
	}
}

// Shuffle performs an in-place Fisher-Yates shuffle in descending index order.
// The direction is pinned: ascending would produce a different permutation for
// the same stream.
func (r *PRNG) Shuffle(n int, swap func(i, j int)) {
	for i := n - 1; i > 0; i-- {
		j := int(r.NextBelow(uint32(i + 1)))
		swap(i, j)
	}
}

// Pick returns a uniformly chosen element.
func Pick[T any](r *PRNG, a []T) T {
	if len(a) == 0 {
		var zero T
		return zero
	}
	return a[r.NextBelow(uint32(len(a)))]
}
