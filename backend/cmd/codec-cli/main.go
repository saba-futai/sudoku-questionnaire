// Command codec-cli exposes the Go codec over stdin/stdout so the TypeScript
// mirror can be compared against it on inputs that the pinned vectors do not
// cover.
//
// The conformance test asserts three things with this binary:
//
//  1. both implementations encode the SAME bytes for the same (key, layout,
//     seed, plaintext) — stronger than a round-trip, because it also catches a
//     divergence where each side is self-consistent but different;
//  2. the TypeScript decoder accepts Go-produced ciphertext;
//  3. the Go decoder accepts TypeScript-produced ciphertext.
//
// Usage:
//
//	codec-cli -mode encode -key K -layout ascii -max-pad 0 -seed <hex>  < plain > wire
//	codec-cli -mode decode -key K -layout ascii                        < wire  > plain
//	codec-cli -mode fingerprint -key K                                 (prints hex)
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"qrforqa/internal/codec"
)

func main() {
	key := flag.String("key", "", "derivation key")
	layoutName := flag.String("layout", "ascii", "wire layout: ascii|entropy")
	maxPad := flag.Int("max-pad", 0, "maximum padding bytes before each clue")
	seedHex := flag.String("seed", "", "u64 encoder seed (hex, required for encode)")
	mode := flag.String("mode", "encode", "encode|decode|fingerprint")
	flag.Parse()

	layoutID, err := codec.ParseLayoutID(*layoutName)
	if err != nil {
		fail(err)
	}
	v, err := codec.EmbeddedValidity()
	if err != nil {
		fail(err)
	}
	table, err := codec.BuildTable(*key, layoutID, v)
	if err != nil {
		fail(err)
	}

	switch *mode {
	case "fingerprint":
		fmt.Println(table.Fingerprint())

	case "encode":
		if *seedHex == "" {
			fail(fmt.Errorf("-seed is required for encode (determinism is the point)"))
		}
		seed, err := parseSeed(*seedHex)
		if err != nil {
			fail(err)
		}
		plain, err := io.ReadAll(os.Stdin)
		if err != nil {
			fail(err)
		}
		write(codec.Encode(table, codec.NewPRNG(seed), plain, *maxPad))

	case "decode":
		wire, err := io.ReadAll(os.Stdin)
		if err != nil {
			fail(err)
		}
		plain, err := codec.Decode(table, wire)
		if err != nil {
			fail(err)
		}
		write(plain)

	default:
		fail(fmt.Errorf("unknown mode %q", *mode))
	}
}

func parseSeed(s string) (uint64, error) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		s = s[2:]
	}
	if len(s) == 16 {
		if v, err := strconv.ParseUint(s, 16, 64); err == nil {
			return v, nil
		}
	}
	if v, err := strconv.ParseUint(s, 10, 64); err == nil {
		return v, nil
	}
	return 0, fmt.Errorf("unparseable seed %q", s)
}

func write(b []byte) {
	if _, err := os.Stdout.Write(b); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "codec-cli: %v\n", err)
	os.Exit(1)
}
