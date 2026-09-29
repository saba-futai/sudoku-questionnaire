// Command genvectors regenerates the codec's shared artifacts:
//
//   - internal/codec/tables/validity.bin   the key-independent validity bitmap
//   - internal/codec/spec_pins.go          the pinned derivation constants
//   - spec/vectors/*.json                  cross-language conformance vectors
//
// Run from the backend directory. The generated files are consumed by both the
// Go tests and the TypeScript mirror, so regenerating is how a deliberate codec
// change is propagated; `make conformance` fails the build if the checked-in
// vectors no longer match.
package main

import (
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"go/format"
	"log"
	"os"
	"path/filepath"

	"qrforqa/internal/codec"
)

type prngCase struct {
	Key          string   `json:"key"`
	TableSeedHex string   `json:"tableSeedHex"`
	NextUint64   []string `json:"nextUint64"`
	NextUint32   []uint32 `json:"nextUint32"`
}

// Byte fields are emitted as hex strings. encoding/json would render []uint8
// as base64, which makes the artifact unreadable and invites a decoder
// mismatch on the consuming side.
type structureVector struct {
	GridCount    int    `json:"gridCount"`
	GridsSHA256  string `json:"gridsSha256"`
	GridsFirst   string `json:"gridsFirstHex"`
	GridsLast    string `json:"gridsLastHex"`
	PosSetCount  int    `json:"posSetCount"`
	PosSetsSHA   string `json:"posSetsSha256"`
	PosSetsFirst string `json:"posSetsFirstHex"`
	PosSetsLast  string `json:"posSetsLastHex"`
}

type validityVector struct {
	SHA256         string         `json:"sha256"`
	Bytes          int            `json:"bytes"`
	PerGridCounts  []int          `json:"perGridCounts"`
	Histogram      map[string]int `json:"histogram"`
	Grid0Positions []int          `json:"grid0Positions"`
}

type layoutVector struct {
	Hint    []string `json:"hint"`
	Padding []string `json:"padding"`
}

type tableCase struct {
	Key           string   `json:"key"`
	Layout        string   `json:"layout"`
	Fingerprint   string   `json:"fingerprint"`
	DecodeMapSize int      `json:"decodeMapSize"`
	Counts        []int    `json:"counts"`
	LayoutSHA     string   `json:"layoutSha256"`
	HintHex       []string `json:"hintAlphabet"`
}

type e2eCase struct {
	Name         string `json:"name"`
	Key          string `json:"key"`
	Layout       string `json:"layout"`
	MaxPad       int    `json:"maxPad"`
	SeedHex      string `json:"seedHex"`
	PlaintextHex string `json:"plaintextHex"`
	WireHex      string `json:"wireHex"`
	WireSHA256   string `json:"wireSha256"`
}

func main() {
	var (
		bitmapPath  = flag.String("bitmap", filepath.Join("internal", "codec", "tables", "validity.bin"), "validity bitmap output")
		pinsPath    = flag.String("pins", filepath.Join("internal", "codec", "spec_pins.go"), "pinned constants output")
		vectorsDir  = flag.String("vectors", filepath.Join("..", "spec", "vectors"), "vector JSON output directory")
		skipBitmap  = flag.Bool("skip-bitmap", false, "do not rewrite the bitmap (reuse the embedded one)")
		deepVerify  = flag.Bool("verify", true, "recompute the bitmap and assert it matches the embedded one")
		vectorCheck = flag.Bool("print-only", false, "print the pins without writing files")
	)
	flag.Parse()

	log.SetFlags(0)
	log.SetPrefix("genvectors: ")

	grids := codec.Grids()
	posSets := codec.PosSets()
	if len(grids) != codec.GridCount || len(posSets) != codec.PosSetCount {
		log.Fatalf("unexpected structure: %d grids, %d position sets", len(grids), len(posSets))
	}

	v := codec.ComputeValidity()
	validitySHA := codec.SHA256Hex(v.Bytes())
	counts := v.Counts()
	histogram := map[string]int{}
	for _, c := range counts {
		histogram[fmt.Sprint(c)]++
	}

	selfTable, err := codec.BuildTable(codec.SelfCheckKey, codec.LayoutASCII, v)
	if err != nil {
		log.Fatalf("build selftest table: %v", err)
	}
	selfWire := codec.Encode(selfTable, codec.NewPRNG(codec.SelfCheckSeed), []byte(codec.SelfCheckProbe), 0)

	pins := struct {
		Grids, PosSets, Validity, Wire string
	}{
		Grids:    codec.GridsSHA256(),
		PosSets:  codec.PosSetsSHA256(),
		Validity: validitySHA,
		Wire:     hex.EncodeToString(selfWire),
	}

	fmt.Printf("grids            %s  (%d)\n", pins.Grids, len(grids))
	fmt.Printf("position sets    %s  (%d)\n", pins.PosSets, len(posSets))
	fmt.Printf("validity bitmap  %s  (%d bytes)\n", pins.Validity, len(v.Bytes()))
	fmt.Printf("per-grid counts  %v\n", histogram)
	fmt.Printf("selftest wire    %s\n", pins.Wire)

	if *vectorCheck {
		return
	}

	if *deepVerify {
		if err := codec.DeepSelfTest(); err != nil {
			log.Fatalf("embedded bitmap disagrees with computation: %v", err)
		}
		fmt.Printf("deep verify      OK\n")
	}

	if !*skipBitmap {
		if err := os.MkdirAll(filepath.Dir(*bitmapPath), 0o755); err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(*bitmapPath, v.Bytes(), 0o644); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("wrote            %s\n", *bitmapPath)
	}

	if err := writePins(*pinsPath, pins.Grids, pins.PosSets, pins.Validity, pins.Wire); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("wrote            %s\n", *pinsPath)

	if err := writeVectors(*vectorsDir, grids, posSets, v); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("wrote            %s\n", *vectorsDir)
}

func writePins(path, gridSHA, posSHA, validitySHA, wire string) error {
	src := fmt.Sprintf(`// Code generated by cmd/genvectors. DO NOT EDIT.
//
// These constants pin the codec's derivation. The TypeScript mirror asserts the
// same values in frontend/src/codec/__tests__, so a change to either
// implementation that alters the derived table cannot be committed without
// regenerating both sides.

package codec

const (
	// PinnedGridsSHA256 hashes the 288 grids in enumeration order.
	PinnedGridsSHA256 = %q
	// PinnedPosSetsSHA256 hashes the 1820 position sets.
	PinnedPosSetsSHA256 = %q
	// PinnedValiditySHA256 hashes the validity bitmap artifact.
	PinnedValiditySHA256 = %q
	// PinnedSelfCheckWire is the ciphertext of the self-test probe.
	PinnedSelfCheckWire = %q
)
`, gridSHA, posSHA, validitySHA, wire)

	formatted, err := format.Source([]byte(src))
	if err != nil {
		return fmt.Errorf("format pins: %w", err)
	}
	return os.WriteFile(path, formatted, 0o644)
}

func writeVectors(dir string, grids []codec.Grid, posSets []codec.PosSet, v *codec.Validity) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	// --- PRNG -------------------------------------------------------------
	keys := []string{"test-key-1", "", "键-🔑-key"}
	prngCases := make([]prngCase, 0, len(keys))
	for _, k := range keys {
		seed := codec.TableSeed(k)
		r := codec.NewPRNG(seed)
		c := prngCase{Key: k, TableSeedHex: fmt.Sprintf("%016x", seed)}
		for i := 0; i < 16; i++ {
			c.NextUint64 = append(c.NextUint64, fmt.Sprintf("%016x", r.NextUint64()))
		}
		r32 := codec.NewPRNG(seed)
		for i := 0; i < 16; i++ {
			c.NextUint32 = append(c.NextUint32, r32.NextUint32())
		}
		prngCases = append(prngCases, c)
	}
	// nextBelow boundary coverage: 1820 (not a power of two) and 288.
	below := map[string][]uint32{}
	for _, n := range []uint32{1820, 288, 24, 7} {
		r := codec.NewPRNG(0x0123456789ABCDEF)
		vals := make([]uint32, 32)
		for i := range vals {
			vals[i] = r.NextBelow(n)
		}
		below[fmt.Sprint(n)] = vals
	}
	if err := writeJSON(filepath.Join(dir, "prng.json"), map[string]any{
		"specVersion": 1,
		"seedDomain":  codec.SeedDomain,
		"cases":       prngCases,
		"nextBelow":   below,
	}); err != nil {
		return err
	}

	// --- Structure --------------------------------------------------------
	last := posSets[len(posSets)-1]
	if err := writeJSON(filepath.Join(dir, "structure.json"), structureVector{
		GridCount:    len(grids),
		GridsSHA256:  codec.GridsSHA256(),
		GridsFirst:   hex.EncodeToString(grids[0][:]),
		GridsLast:    hex.EncodeToString(grids[len(grids)-1][:]),
		PosSetCount:  len(posSets),
		PosSetsSHA:   codec.PosSetsSHA256(),
		PosSetsFirst: hex.EncodeToString(posSets[0][:]),
		PosSetsLast:  hex.EncodeToString(last[:]),
	}); err != nil {
		return err
	}

	// --- Validity ---------------------------------------------------------
	row0 := []int{}
	for p := 0; p < codec.PosSetCount; p++ {
		if v.Valid(0, p) {
			row0 = append(row0, p)
		}
	}
	counts := v.Counts()
	histogram := map[string]int{}
	for _, c := range counts {
		histogram[fmt.Sprint(c)]++
	}
	if err := writeJSON(filepath.Join(dir, "validity.json"), validityVector{
		SHA256:         codec.SHA256Hex(v.Bytes()),
		Bytes:          len(v.Bytes()),
		PerGridCounts:  counts,
		Histogram:      histogram,
		Grid0Positions: row0,
	}); err != nil {
		return err
	}

	// --- Layouts ----------------------------------------------------------
	layouts := map[string]layoutVector{}
	for _, id := range []codec.LayoutID{codec.LayoutASCII, codec.LayoutEntropy} {
		l := codec.NewLayout(id)
		lv := layoutVector{}
		for g := 0; g < 64; g++ {
			lv.Hint = append(lv.Hint, fmt.Sprintf("%02x", l.HintByte(byte(g))))
		}
		for _, p := range l.PaddingPool() {
			lv.Padding = append(lv.Padding, fmt.Sprintf("%02x", p))
		}
		layouts[id.String()] = lv
	}
	if err := writeJSON(filepath.Join(dir, "layouts.json"), map[string]any{
		"specVersion": 1,
		"layouts":     layouts,
	}); err != nil {
		return err
	}

	// --- Tables -----------------------------------------------------------
	tableCases := []tableCase{}
	for _, k := range keys {
		for _, id := range []codec.LayoutID{codec.LayoutASCII, codec.LayoutEntropy} {
			t, err := codec.BuildTable(k, id, v)
			if err != nil {
				return err
			}
			l := t.Layout()
			hint := make([]string, 64)
			alphabet := make([]byte, 0, 64+len(l.PaddingPool()))
			for g := 0; g < 64; g++ {
				hint[g] = fmt.Sprintf("%02x", l.HintByte(byte(g)))
				alphabet = append(alphabet, l.HintByte(byte(g)))
			}
			alphabet = append(alphabet, l.PaddingPool()...)
			cs := t.EncodeCounts()
			tableCases = append(tableCases, tableCase{
				Key:           k,
				Layout:        id.String(),
				Fingerprint:   t.Fingerprint(),
				DecodeMapSize: t.DecodeMapSize(),
				Counts:        cs[:],
				LayoutSHA:     codec.SHA256Hex(alphabet),
				HintHex:       hint,
			})
		}
	}
	if err := writeJSON(filepath.Join(dir, "table.json"), map[string]any{
		"specVersion": 1,
		"cases":       tableCases,
	}); err != nil {
		return err
	}

	// --- End to end -------------------------------------------------------
	e2e := []e2eCase{}
	plaintexts := []struct {
		name string
		body []byte
	}{
		{"empty", []byte{}},
		{"hi", []byte("hi")},
		{"all256", seqBytes(256)},
		{"pattern1000", patternBytes(1000)},
	}
	seed := uint64(0x1122334455667788)
	for _, lay := range []codec.LayoutID{codec.LayoutASCII, codec.LayoutEntropy} {
		for _, maxPad := range []int{0, 3} {
			for _, pt := range plaintexts {
				t, err := codec.BuildTable("test-key-1", lay, v)
				if err != nil {
					return err
				}
				wire := codec.Encode(t, codec.NewPRNG(seed), pt.body, maxPad)
				e2e = append(e2e, e2eCase{
					Name:         fmt.Sprintf("%s/pad%d/%s", lay, maxPad, pt.name),
					Key:          "test-key-1",
					Layout:       lay.String(),
					MaxPad:       maxPad,
					SeedHex:      fmt.Sprintf("%016x", seed),
					PlaintextHex: hex.EncodeToString(pt.body),
					WireHex:      hex.EncodeToString(wire),
					WireSHA256:   codec.SHA256Hex(wire),
				})
			}
		}
	}
	// A non-ASCII key exercises UTF-8 key hashing, the classic silent-divergence trap.
	t, err := codec.BuildTable("键-🔑-key", codec.LayoutASCII, v)
	if err != nil {
		return err
	}
	body := []byte("中文答案 with mixed 🔑 content")
	wire := codec.Encode(t, codec.NewPRNG(seed), body, 0)
	e2e = append(e2e, e2eCase{
		Name:         "unicode-key/ascii/pad0/utf8",
		Key:          "键-🔑-key",
		Layout:       "ascii",
		MaxPad:       0,
		SeedHex:      fmt.Sprintf("%016x", seed),
		PlaintextHex: hex.EncodeToString(body),
		WireHex:      hex.EncodeToString(wire),
		WireSHA256:   codec.SHA256Hex(wire),
	})
	if err := writeJSON(filepath.Join(dir, "e2e.json"), map[string]any{
		"specVersion": 1,
		"cases":       e2e,
	}); err != nil {
		return err
	}

	return nil
}

func seqBytes(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i)
	}
	return b
}

func patternBytes(n int) []byte {
	b := make([]byte, n)
	x := uint32(0x12345678)
	for i := range b {
		x = x*1664525 + 1013904223
		b[i] = byte(x >> 24)
	}
	return b
}

func writeJSON(path string, v any) error {
	buf, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}
	buf = append(buf, '\n')
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		return err
	}
	return nil
}
