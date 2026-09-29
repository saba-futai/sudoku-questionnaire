package codec

import (
	"bytes"
	"errors"
	"math/rand"
	"strings"
	"testing"
)

func mustValidity(t *testing.T) *Validity {
	t.Helper()
	v, err := EmbeddedValidity()
	if err != nil {
		t.Fatalf("EmbeddedValidity: %v", err)
	}
	return v
}

func mustTable(t *testing.T, key string, id LayoutID) *Table {
	t.Helper()
	tb, err := BuildTable(key, id, mustValidity(t))
	if err != nil {
		t.Fatalf("BuildTable(%q,%s): %v", key, id, err)
	}
	return tb
}

func TestStructure(t *testing.T) {
	if got := len(Grids()); got != GridCount {
		t.Errorf("grids: got %d want %d", got, GridCount)
	}
	if got := len(PosSets()); got != PosSetCount {
		t.Errorf("position sets: got %d want %d", got, PosSetCount)
	}
	if PosSets()[0] != (PosSet{0, 1, 2, 3}) {
		t.Errorf("first position set: %v", PosSets()[0])
	}
	if last := PosSets()[PosSetCount-1]; last != (PosSet{12, 13, 14, 15}) {
		t.Errorf("last position set: %v", last)
	}
	// Ascending order is required by the lexicographic enumeration contract.
	for i, p := range PosSets() {
		if !(p[0] < p[1] && p[1] < p[2] && p[2] < p[3]) {
			t.Fatalf("position set %d not ascending: %v", i, p)
		}
	}
}

// Every grid must be a legal sudoku: rows, columns and 2x2 boxes each a
// permutation of 1..4.
func TestGridsAreValidSudoku(t *testing.T) {
	for gi, g := range Grids() {
		var rows, cols, boxes [4]uint8
		for i := 0; i < 16; i++ {
			bit := uint8(1) << (g[i] - 1)
			if g[i] < 1 || g[i] > 4 {
				t.Fatalf("grid %d cell %d out of range: %d", gi, i, g[i])
			}
			rows[i/4] |= bit
			cols[i%4] |= bit
			boxes[(i/4/2)*2+i%4/2] |= bit
		}
		for k := 0; k < 4; k++ {
			if rows[k] != 0xF || cols[k] != 0xF || boxes[k] != 0xF {
				t.Fatalf("grid %d is not a valid sudoku (row/col/box %d)", gi, k)
			}
		}
	}
}

// The shipped bitmap must equal a fresh computation. This is what makes the
// build-time artifact safe to embed: a stale or corrupted file fails here
// rather than silently deriving a table only some inputs decode against.
func TestEmbeddedValidityMatchesComputation(t *testing.T) {
	if err := DeepSelfTest(); err != nil {
		t.Fatal(err)
	}
}

// The 96/192 split is the signature of the two symmetry orbits of 4x4 sudoku.
// Any change to the grid traversal or the position-set order perturbs it, so it
// doubles as a tripwire on both enumerations.
func TestValidityOrbitStructure(t *testing.T) {
	hist := map[int]int{}
	for _, c := range mustValidity(t).Counts() {
		hist[c]++
	}
	want := map[int]int{12: 96, 128: 192}
	if len(hist) != len(want) {
		t.Fatalf("count histogram: got %v want %v", hist, want)
	}
	for k, v := range want {
		if hist[k] != v {
			t.Errorf("grids with %d encodings: got %d want %d", k, hist[k], v)
		}
	}
}

func TestEveryByteEncodable(t *testing.T) {
	tb := mustTable(t, "encodable", LayoutASCII)
	for b := 0; b < 256; b++ {
		if tb.EncodingCount(byte(b)) == 0 {
			t.Fatalf("byte %d has no encoding", b)
		}
	}
}

func TestLayoutBijections(t *testing.T) {
	for _, id := range []LayoutID{LayoutASCII, LayoutEntropy} {
		l := NewLayout(id)
		if err := l.ValidateLayout(); err != nil {
			t.Fatal(err)
		}
		// Every group must reach a distinct wire byte in the alphabet.
		seen := map[byte]bool{}
		for g := 0; g < 64; g++ {
			b := l.HintByte(byte(g))
			if !l.IsHint(b) {
				t.Fatalf("%s: group %d maps to 0x%02x which is not a clue byte", id, g, b)
			}
			if seen[b] {
				t.Fatalf("%s: wire byte 0x%02x reused", id, b)
			}
			seen[b] = true
			back, ok := l.GroupOf(b)
			if !ok || back != byte(g) {
				t.Fatalf("%s: group %d round-trip gave %d,%v", id, g, back, ok)
			}
		}
		if len(seen) != 64 {
			t.Fatalf("%s: alphabet has %d symbols, want 64", id, len(seen))
		}
	}
}

// Upstream's ASCII predicate `(b & 0x40) == 0x40` also accepts 0x80..0xFF, so a
// stray UTF-8 byte would be silently eaten as a clue. The strict predicate must
// reject it.
func TestASCIILayoutRejectsHighBytes(t *testing.T) {
	l := NewLayout(LayoutASCII)
	for _, b := range []byte{0x00, 0x19, 0x1F, 0x7F, 0x80, 0xA9, 0xC0, 0xE4, 0xFF} {
		if l.IsHint(b) {
			t.Errorf("0x%02x wrongly accepted as a clue byte", b)
		}
		if l.IsPadding(b) {
			t.Errorf("0x%02x wrongly accepted as padding", b)
		}
		if _, ok := l.GroupOf(b); ok {
			t.Errorf("0x%02x wrongly decoded as a group", b)
		}
	}
	// The 64-symbol alphabet is exactly printable 0x40..0x7E plus the escaped newline.
	if !l.IsHint('\n') || !l.IsHint(0x40) || !l.IsHint(0x7E) || l.IsHint(0x7F) {
		t.Error("ascii alphabet boundaries are wrong")
	}
}

func TestRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewSource(20260929))
	keys := []string{"test-key-1", "", "键-🔑-key", "k"}
	sizes := []int{0, 1, 2, 3, 4, 5, 63, 64, 65, 255, 256, 1000, 4096}

	for _, key := range keys {
		for _, id := range []LayoutID{LayoutASCII, LayoutEntropy} {
			tb := mustTable(t, key, id)
			for _, size := range sizes {
				plain := make([]byte, size)
				rng.Read(plain)
				for _, maxPad := range []int{0, 3, 10} {
					cipher := Encode(tb, NewPRNG(rng.Uint64()), plain, maxPad)
					got, err := Decode(tb, cipher)
					if err != nil {
						t.Fatalf("key=%q layout=%s size=%d pad=%d: %v", key, id, size, maxPad, err)
					}
					if !bytes.Equal(got, plain) {
						t.Fatalf("key=%q layout=%s size=%d pad=%d: round-trip mismatch", key, id, size, maxPad)
					}
				}
			}
		}
	}
}

// A fresh table instance from the same key must decode, which is exactly the
// browser -> server path where the two sides never share memory.
func TestCrossInstanceRoundTrip(t *testing.T) {
	plain := []byte("the same key on both sides is the whole contract")
	sender := mustTable(t, "shared-key", LayoutASCII)
	receiver := mustTable(t, "shared-key", LayoutASCII)
	if sender.Fingerprint() != receiver.Fingerprint() {
		t.Fatal("same key produced different tables")
	}
	cipher := Encode(sender, NewPRNG(7), plain, 0)
	got, err := Decode(receiver, cipher)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatal("cross-instance round-trip mismatch")
	}
}

func TestEncodeIsDeterministicForAGivenSeed(t *testing.T) {
	tb := mustTable(t, "determinism", LayoutASCII)
	plain := []byte("repeatable")
	a := Encode(tb, NewPRNG(0xABCDEF), plain, 2)
	b := Encode(tb, NewPRNG(0xABCDEF), plain, 2)
	if !bytes.Equal(a, b) {
		t.Fatal("same seed produced different ciphertext")
	}
}

// Identical answers must not produce identical ciphertext, or equal answers
// across respondents would be linkable by anyone reading the database.
func TestEncodeVariesAcrossSeeds(t *testing.T) {
	tb := mustTable(t, "obfuscation", LayoutASCII)
	plain := []byte("yes")
	distinct := map[string]bool{}
	for i := 0; i < 64; i++ {
		distinct[string(Encode(tb, NewPRNG(uint64(i)+1), plain, 0))] = true
	}
	// "yes" is 3 bytes; with 12+ puzzles each and 24 permutations, 64 draws
	// should almost never collapse to a handful of forms.
	if len(distinct) < 40 {
		t.Fatalf("only %d distinct ciphertexts from 64 seeds; obfuscation is degenerate", len(distinct))
	}
}

func TestDecodeRejectsBytesOutsideAlphabet(t *testing.T) {
	tb := mustTable(t, "strict", LayoutASCII)
	cipher := Encode(tb, NewPRNG(1), []byte("hello"), 0)
	bad := append([]byte{}, cipher...)
	bad[2] = 0xFF // neither a clue byte nor padding
	_, err := Decode(tb, bad)
	if !errors.Is(err, ErrInvalidWireByte) {
		t.Fatalf("got %v, want ErrInvalidWireByte", err)
	}
}

// Corrupting the checksum trailer must be caught. Note this targets the
// TRAILER specifically, not a body clue: replacing a body clue byte merely
// selects a different valid puzzle for the same plaintext byte, which is a
// legitimate alternative ciphertext and must NOT be reported as corruption.
func TestDecodeDetectsChecksumCorruption(t *testing.T) {
	lay := NewLayout(LayoutASCII)
	tb := mustTable(t, "tamper", LayoutASCII)
	plain := []byte("immutable record")

	for delta := 1; delta <= 4; delta++ {
		cipher := Encode(tb, NewPRNG(uint64(delta)), plain, 0)

		// Read the current trailer, then write back a provably different value.
		hints := collectHints(t, tb, cipher)
		n := len(hints)/4 - 1
		var want uint32
		for i := 0; i < trailerBytes; i++ {
			want = want<<6 | uint32(hints[n*4+i]&0x3F)
		}
		alt := (want + uint32(delta)) & 0xFFFFFF

		bad := append([]byte{}, cipher...)
		base := n * 4
		for i := 0; i < trailerBytes; i++ {
			bad[base+i] = lay.HintByte(byte(alt>>uint(18-6*i)) & 0x3F)
		}
		if _, err := Decode(tb, bad); !errors.Is(err, ErrChecksumMismatch) {
			t.Fatalf("delta=%d: got %v, want ErrChecksumMismatch", delta, err)
		}
	}
}

// Removing a clue symbol must be caught as a frame error.
func TestDecodeDetectsRemovedClue(t *testing.T) {
	tb := mustTable(t, "drop", LayoutASCII)
	cipher := Encode(tb, NewPRNG(1), []byte("drop a clue"), 0)
	bad := append(append([]byte{}, cipher[:5]...), cipher[6:]...)
	if _, err := Decode(tb, bad); !errors.Is(err, ErrBadFrame) {
		t.Fatalf("got %v, want ErrBadFrame", err)
	}
}

// collectHints returns the group sequence the decoder would see.
func collectHints(t *testing.T, tb *Table, wire []byte) []byte {
	t.Helper()
	lay := tb.Layout()
	out := make([]byte, 0, len(wire))
	for _, b := range wire {
		if g, ok := lay.GroupOf(b); ok {
			out = append(out, g)
		}
	}
	return out
}

func TestDecodeDetectsTruncation(t *testing.T) {
	tb := mustTable(t, "truncate", LayoutASCII)
	cipher := Encode(tb, NewPRNG(3), []byte("a longer sentence to truncate somewhere inside"), 0)
	for _, cut := range []int{0, 1, 4, len(cipher) / 2, len(cipher) - 1} {
		if _, err := Decode(tb, cipher[:cut]); err == nil {
			t.Errorf("truncation to %d bytes decoded without error", cut)
		}
	}
}

func TestDecodeRejectsWrongKey(t *testing.T) {
	plain := []byte("secret answer")
	cipher := Encode(mustTable(t, "right-key", LayoutASCII), NewPRNG(5), plain, 0)
	if _, err := Decode(mustTable(t, "wrong-key", LayoutASCII), cipher); err == nil {
		t.Fatal("decoded with the wrong key")
	}
}

func TestDecodeRejectsWrongLayout(t *testing.T) {
	plain := []byte("layout specific")
	cipher := Encode(mustTable(t, "layout-key", LayoutASCII), NewPRNG(5), plain, 0)
	if _, err := Decode(mustTable(t, "layout-key", LayoutEntropy), cipher); err == nil {
		t.Fatal("decoded ASCII ciphertext with the entropy layout")
	}
}

// Padding must be invisible to the decoder, including when it precedes the
// checksum trailer.
func TestPaddingIsTransparent(t *testing.T) {
	tb := mustTable(t, "padding", LayoutASCII)
	plain := []byte("padded payload")
	base := Encode(tb, NewPRNG(11), plain, 0)
	for _, maxPad := range []int{0, 1, 3, 8} {
		cipher := Encode(tb, NewPRNG(11), plain, maxPad)
		if len(cipher) < len(base) {
			t.Fatalf("maxPad=%d produced a shorter ciphertext than unpadded", maxPad)
		}
		got, err := Decode(tb, cipher)
		if err != nil {
			t.Fatalf("maxPad=%d: %v", maxPad, err)
		}
		if !bytes.Equal(got, plain) {
			t.Fatalf("maxPad=%d: mismatch", maxPad)
		}
	}
	if got := len(Encode(tb, NewPRNG(11), plain, 8)); got <= len(base) {
		t.Error("padding did not lengthen the ciphertext at all")
	}
}

// The ASCII layout exists so a ciphertext can be stored as text and read by a
// human; that guarantee must hold for arbitrary input.
func TestASCIICiphertextIsTextSafe(t *testing.T) {
	tb := mustTable(t, "textsafe", LayoutASCII)
	l := tb.Layout()
	rng := rand.New(rand.NewSource(99))
	plain := make([]byte, 2048)
	rng.Read(plain)
	cipher := Encode(tb, NewPRNG(rng.Uint64()), plain, 4)
	for i, b := range cipher {
		if !l.IsHint(b) && !l.IsPadding(b) {
			t.Fatalf("byte %d of ciphertext is 0x%02x, outside the layout alphabet", i, b)
		}
		if b == 0 {
			t.Fatalf("ciphertext contains NUL at %d", i)
		}
	}
}

func TestDecodeEmptyPlaintext(t *testing.T) {
	tb := mustTable(t, "empty", LayoutASCII)
	cipher := Encode(tb, NewPRNG(1), nil, 0)
	if len(cipher) != trailerBytes {
		t.Fatalf("empty plaintext produced %d bytes, want %d", len(cipher), trailerBytes)
	}
	got, err := Decode(tb, cipher)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d bytes, want 0", len(got))
	}
}

func TestFrameSize(t *testing.T) {
	tb := mustTable(t, "frame", LayoutASCII)
	for _, n := range []int{0, 1, 7, 512} {
		cipher := Encode(tb, NewPRNG(2), make([]byte, n), 0)
		if want := n*4 + frameOverheadBytes; len(cipher) != want {
			t.Errorf("n=%d: ciphertext %d bytes, want %d", n, len(cipher), want)
		}
	}
}

func TestInspectExplainsMalformedInput(t *testing.T) {
	tb := mustTable(t, "inspect", LayoutASCII)
	cipher := Encode(tb, NewPRNG(4), []byte("report me"), 2)
	rep := Inspect(tb, cipher)
	if !rep.ChecksumOK {
		t.Error("clean ciphertext reported a bad checksum")
	}
	if string(rep.Plaintext) != "report me" {
		t.Errorf("plaintext: %q", rep.Plaintext)
	}
	if rep.ClueSymbols != 9*4+trailerBytes {
		t.Errorf("clue symbols: %d", rep.ClueSymbols)
	}
	if rep.PaddingBytes == 0 {
		t.Error("expected some padding bytes at maxPad=2")
	}

	bad := append(append([]byte{}, cipher...), 0x01)
	repBad := Inspect(tb, bad)
	if repBad.RejectedByte != 0x01 {
		t.Errorf("rejected byte: got %d want 1", repBad.RejectedByte)
	}
}

func TestSelfTestPasses(t *testing.T) {
	if err := SelfTest(); err != nil {
		t.Fatal(err)
	}
}

func TestFingerprintIsLayoutIndependent(t *testing.T) {
	// The fingerprint identifies the key derivation, which does not depend on
	// how groups are painted onto bytes.
	a := mustTable(t, "fp", LayoutASCII).Fingerprint()
	b := mustTable(t, "fp", LayoutEntropy).Fingerprint()
	if a != b {
		t.Error("fingerprint should not depend on the wire layout")
	}
	if a == mustTable(t, "other", LayoutASCII).Fingerprint() {
		t.Error("different keys produced the same fingerprint")
	}
	if len(a) != 64 || strings.Trim(a, "0123456789abcdef") != "" {
		t.Errorf("fingerprint is not a hex digest: %q", a)
	}
}

func TestNextBelowRangeAndUniformity(t *testing.T) {
	const n = 1820
	r := NewPRNG(0xDEADBEEF)
	buckets := make([]int, n)
	const draws = 200000
	for i := 0; i < draws; i++ {
		v := r.NextBelow(n)
		if v >= n {
			t.Fatalf("NextBelow(%d) returned %d", n, v)
		}
		buckets[v]++
	}
	// Rejection sampling is unbiased; allow generous slack for sampling noise.
	mean := float64(draws) / n
	for i, c := range buckets {
		if float64(c) < mean*0.6 || float64(c) > mean*1.4 {
			t.Fatalf("bucket %d has %d draws, expected about %.0f", i, c, mean)
		}
	}
	if got := r.NextBelow(1); got != 0 {
		t.Errorf("NextBelow(1) = %d, want 0", got)
	}
}

func TestShuffleIsAPermutation(t *testing.T) {
	a := make([]int, GridCount)
	for i := range a {
		a[i] = i
	}
	NewPRNG(42).Shuffle(len(a), func(i, j int) { a[i], a[j] = a[j], a[i] })
	seen := make([]bool, GridCount)
	for _, v := range a {
		if v < 0 || v >= GridCount || seen[v] {
			t.Fatalf("not a permutation: %v", a)
		}
		seen[v] = true
	}
	// Different seeds must give different orders.
	b := make([]int, GridCount)
	for i := range b {
		b[i] = i
	}
	NewPRNG(43).Shuffle(len(b), func(i, j int) { b[i], b[j] = b[j], b[i] })
	same := true
	for i := range a {
		if a[i] != b[i] {
			same = false
			break
		}
	}
	if same {
		t.Error("shuffle produced identical orders for different seeds")
	}
}

func BenchmarkEncode4KB(b *testing.B) {
	v, err := EmbeddedValidity()
	if err != nil {
		b.Fatal(err)
	}
	tb, err := BuildTable("bench", LayoutASCII, v)
	if err != nil {
		b.Fatal(err)
	}
	plain := make([]byte, 4096)
	b.SetBytes(int64(len(plain)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Encode(tb, NewPRNG(uint64(i)+1), plain, 0)
	}
}

func BenchmarkDecode4KB(b *testing.B) {
	v, err := EmbeddedValidity()
	if err != nil {
		b.Fatal(err)
	}
	tb, err := BuildTable("bench", LayoutASCII, v)
	if err != nil {
		b.Fatal(err)
	}
	cipher := Encode(tb, NewPRNG(1), make([]byte, 4096), 0)
	b.SetBytes(4096)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Decode(tb, cipher); err != nil {
			b.Fatal(err)
		}
	}
}
