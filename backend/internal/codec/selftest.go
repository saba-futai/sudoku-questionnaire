package codec

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// Self-test inputs. The probe exercises the whole pipeline — seed derivation,
// shuffle, table build, layout, permutation, trailer — in about a millisecond,
// so a build-time regression fails at startup instead of corrupting the first
// questionnaire that arrives.
const (
	SelfCheckKey   = "selftest"
	SelfCheckProbe = "probe"
	SelfCheckSeed  = 0xDEADBEEFCAFEBABE
)

// SHA256Hex is a small helper shared by self-test and vector generation.
func SHA256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// SelfTest verifies the codec's structural invariants and, when the pinned
// constants have been generated, its exact derivation.
//
// Called on server start and from the frontend's boot path. Returning an error
// must abort startup: every failure mode here means the server would derive a
// table the browser cannot read, or would silently mis-decode submissions.
func SelfTest() error {
	if got := len(Grids()); got != GridCount {
		return fmt.Errorf("selftest: %d grids, want %d", got, GridCount)
	}
	if got := len(PosSets()); got != PosSetCount {
		return fmt.Errorf("selftest: %d position sets, want %d", got, PosSetCount)
	}
	if PinnedGridsSHA256 != "" {
		if got := GridsSHA256(); got != PinnedGridsSHA256 {
			return fmt.Errorf("selftest: grid enumeration drifted: %s != pinned %s", got, PinnedGridsSHA256)
		}
	}
	if PinnedPosSetsSHA256 != "" {
		if got := PosSetsSHA256(); got != PinnedPosSetsSHA256 {
			return fmt.Errorf("selftest: position-set enumeration drifted: %s != pinned %s", got, PinnedPosSetsSHA256)
		}
	}

	v, err := EmbeddedValidity()
	if err != nil {
		return fmt.Errorf("selftest: embedded validity bitmap unusable: %w", err)
	}
	if PinnedValiditySHA256 != "" {
		if got := SHA256Hex(v.Bytes()); got != PinnedValiditySHA256 {
			return fmt.Errorf("selftest: validity bitmap drifted: %s != pinned %s", got, PinnedValiditySHA256)
		}
	}
	for g := 0; g < GridCount; g++ {
		if v.EncodeCount(g) == 0 {
			return fmt.Errorf("selftest: grid %d has no uniquely-identifying clue set", g)
		}
	}

	for _, id := range []LayoutID{LayoutASCII, LayoutEntropy} {
		if err := NewLayout(id).ValidateLayout(); err != nil {
			return fmt.Errorf("selftest: %w", err)
		}
	}

	t, err := BuildTable(SelfCheckKey, LayoutASCII, v)
	if err != nil {
		return fmt.Errorf("selftest: %w", err)
	}
	for b := 0; b < 256; b++ {
		if t.EncodingCount(byte(b)) == 0 {
			return fmt.Errorf("selftest: byte %d has no encoding", b)
		}
	}

	wire := Encode(t, NewPRNG(SelfCheckSeed), []byte(SelfCheckProbe), 0)
	if PinnedSelfCheckWire != "" {
		if got := hex.EncodeToString(wire); got != PinnedSelfCheckWire {
			return fmt.Errorf("selftest: codec derivation drifted: %s != pinned %s", got, PinnedSelfCheckWire)
		}
	}
	back, err := Decode(t, wire)
	if err != nil {
		return fmt.Errorf("selftest: decode of own output failed: %w", err)
	}
	if string(back) != SelfCheckProbe {
		return fmt.Errorf("selftest: round-trip mismatch: %q", back)
	}
	return nil
}

// DeepSelfTest additionally recomputes the validity bitmap from scratch and
// compares it against the shipped artifact. It costs a few hundred milliseconds
// and is therefore run by tests and by an explicit operator flag, not on every
// boot.
func DeepSelfTest() error {
	v, err := EmbeddedValidity()
	if err != nil {
		return err
	}
	fresh := ComputeValidity()
	if SHA256Hex(fresh.Bytes()) != SHA256Hex(v.Bytes()) {
		return fmt.Errorf("selftest: embedded validity bitmap disagrees with computation")
	}
	return nil
}
