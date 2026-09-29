package codec

import (
	"crypto/sha256" //nolint:unused // used by EnvelopeHash below
	"encoding/hex"
	"errors"
	"fmt"
	"hash/crc32"
)

// Wire grammar errors. Every one of these is a hard failure: a payload that
// trips any of them is rejected rather than partially decoded, so a wrong key,
// a wrong layout, a truncated body or an edited byte all surface immediately
// instead of turning into plausible-looking garbage.
var (
	ErrInvalidWireByte  = errors.New("sudoku: byte is neither a clue nor padding")
	ErrBadFrame         = errors.New("sudoku: malformed frame")
	ErrUnknownPuzzle    = errors.New("sudoku: clue set maps to no plaintext byte")
	ErrChecksumMismatch = errors.New("sudoku: checksum mismatch")
)

// trailerBytes is the fixed number of clue bytes appended after the body,
// carrying a 24-bit checksum of the plaintext.
const trailerBytes = 4

// Frame overhead in wire symbols: nothing before the body, 4 after.
const frameOverheadBytes = trailerBytes

// MaxPadDefault is the default maximum padding bytes inserted before each clue
// byte. Zero: this application is served over HTTP(S) as ordinary JSON, not
// disguised as DPI-evading tunnel traffic, so padding would only inflate
// storage and bandwidth.
const MaxPadDefault = 0

// Encode maps plaintext to its sudoku ciphertext.
//
// rng supplies the obfuscation stream: which of a byte's valid puzzles to use,
// which of the 24 permutations to emit, and where to sprinkle padding. It must
// be freshly seeded per payload in production so identical answers do not
// produce identical ciphertext; tests pass a fixed seed to get reproducible
// vectors.
func Encode(t *Table, rng *PRNG, plain []byte, maxPad int) []byte {
	lay := t.layout
	out := make([]byte, 0, len(plain)*4+frameOverheadBytes+8)

	for _, b := range plain {
		encs := t.encode[b]
		q := &encs[rng.NextBelow(uint32(len(encs)))]
		perm := &perm4[rng.NextBelow(uint32(len(perm4)))]
		for i := 0; i < 4; i++ {
			out = appendPadding(out, lay, rng, maxPad)
			out = append(out, lay.HintByte(q[perm[i]]))
		}
	}

	// Checksum trailer, emitted positionally (never permuted, never sorted).
	c := crc32.ChecksumIEEE(plain) & 0xFFFFFF
	for shift := 18; shift >= 0; shift -= 6 {
		out = append(out, lay.HintByte(byte(c>>uint(shift))&0x3F))
	}
	return out
}

func appendPadding(out []byte, lay *Layout, rng *PRNG, maxPad int) []byte {
	if maxPad <= 0 {
		return out
	}
	n := int(rng.NextBelow(uint32(maxPad + 1)))
	for i := 0; i < n; i++ {
		out = append(out, Pick(rng, lay.padding))
	}
	return out
}

// Decode recovers plaintext from ciphertext, or returns a wire-grammar error.
//
// Padding is skipped; anything outside the clue alphabet and the padding pool
// is an error. The final four clue symbols are read positionally as the
// checksum, which is what makes the frame self-describing: the plaintext length
// is implied by the clue count rather than carried in a separate field.
func Decode(t *Table, wire []byte) ([]byte, error) {
	lay := t.layout

	hints := make([]byte, 0, len(wire))
	for i := 0; i < len(wire); i++ {
		b := wire[i]
		if g, ok := lay.GroupOf(b); ok {
			hints = append(hints, g)
			continue
		}
		if lay.IsPadding(b) {
			continue
		}
		return nil, fmt.Errorf("%w: 0x%02x at offset %d", ErrInvalidWireByte, b, i)
	}

	if len(hints) < trailerBytes || (len(hints)-trailerBytes)%4 != 0 {
		return nil, fmt.Errorf("%w: %d clue symbols", ErrBadFrame, len(hints))
	}

	n := (len(hints) - trailerBytes) / 4
	out := make([]byte, 0, n)
	for i := 0; i < n; i++ {
		var gr [4]byte
		copy(gr[:], hints[i*4:i*4+4])
		v, ok := t.decode[PackGroups(gr)]
		if !ok {
			return nil, fmt.Errorf("%w: puzzle %d", ErrUnknownPuzzle, i)
		}
		out = append(out, v)
	}

	var want uint32
	for i := 0; i < trailerBytes; i++ {
		want = want<<6 | uint32(hints[n*4+i]&0x3F)
	}
	if got := crc32.ChecksumIEEE(out) & 0xFFFFFF; got != want {
		return nil, fmt.Errorf("%w: got %06x, frame says %06x", ErrChecksumMismatch, got, want)
	}
	return out, nil
}

// EnvelopeHash identifies a ciphertext for dedup/audit purposes.
func EnvelopeHash(wire []byte) string {
	sum := sha256.Sum256(wire)
	return hex.EncodeToString(sum[:])
}

// DecodeReport describes how a ciphertext decomposed. It backs the admin codec
// inspector, which shows the plaintext, the per-byte puzzle choices and the
// checksum side by side.
type DecodeReport struct {
	Plaintext    []byte `json:"plaintext"`
	ClueSymbols  int    `json:"clueSymbols"`
	PaddingBytes int    `json:"paddingBytes"`
	RejectedByte int    `json:"rejectedByte"` // -1 when the frame is clean
	PlaintextLen int    `json:"plaintextLen"`
	ChecksumOK   bool   `json:"checksumOk"`
	TrailerHex   string `json:"trailerHex"`
}

// Inspect decodes without failing fast, so the inspector can explain a
// malformed payload instead of only reporting that it is malformed.
func Inspect(t *Table, wire []byte) DecodeReport {
	lay := t.layout
	rep := DecodeReport{RejectedByte: -1}

	hints := make([]byte, 0, len(wire))
	for _, b := range wire {
		if g, ok := lay.GroupOf(b); ok {
			hints = append(hints, g)
			continue
		}
		if lay.IsPadding(b) {
			rep.PaddingBytes++
			continue
		}
		if rep.RejectedByte < 0 {
			rep.RejectedByte = int(b)
		}
	}
	rep.ClueSymbols = len(hints)
	if len(hints) < trailerBytes || (len(hints)-trailerBytes)%4 != 0 {
		return rep
	}

	n := (len(hints) - trailerBytes) / 4
	out := make([]byte, 0, n)
	for i := 0; i < n; i++ {
		var gr [4]byte
		copy(gr[:], hints[i*4:i*4+4])
		v, ok := t.decode[PackGroups(gr)]
		if !ok {
			return rep
		}
		out = append(out, v)
	}
	var want uint32
	for i := 0; i < trailerBytes; i++ {
		want = want<<6 | uint32(hints[n*4+i]&0x3F)
	}
	rep.Plaintext = out
	rep.PlaintextLen = len(out)
	rep.TrailerHex = fmt.Sprintf("%06x", want)
	rep.ChecksumOK = crc32.ChecksumIEEE(out)&0xFFFFFF == want
	return rep
}
