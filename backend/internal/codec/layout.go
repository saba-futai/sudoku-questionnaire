package codec

import "fmt"

// LayoutID selects the wire alphabet. Layouts are interchangeable: they map
// the same 64 group values onto different byte sets, so the table, the decode
// map and the wire grammar are all layout-independent.
type LayoutID uint8

const (
	// LayoutASCII emits only bytes in {0x40..0x7E} plus '\n', so a ciphertext
	// is safe to store in SQLite TEXT, embed in JSON, and eyeball in a
	// terminal. This is the default; the app has no DPI-evasion requirement
	// that would justify the entropy layout's control bytes.
	LayoutASCII LayoutID = iota
	// LayoutEntropy emits high-entropy bytes, matching upstream's
	// "prefer_entropy" mode. Text-transport unsafe: contains 0x00..0x0F.
	LayoutEntropy
)

func (id LayoutID) String() string {
	switch id {
	case LayoutASCII:
		return "ascii"
	case LayoutEntropy:
		return "entropy"
	default:
		return fmt.Sprintf("layout(%d)", uint8(id))
	}
}

// ParseLayoutID parses the wire name used in storage and API payloads.
func ParseLayoutID(s string) (LayoutID, error) {
	switch s {
	case "ascii", "":
		return LayoutASCII, nil
	case "entropy":
		return LayoutEntropy, nil
	default:
		return 0, fmt.Errorf("unknown layout %q", s)
	}
}

// Layout is a bijection between the 64 group values and 64 wire bytes, plus a
// disjoint set of padding bytes.
type Layout struct {
	id      LayoutID
	hint    [64]byte   // group -> wire byte
	group   [256]byte  // wire byte -> group (meaningful only when isHint)
	isHint  [256]bool  // exact alphabet membership
	padding []byte     // pad pool, disjoint from the hint alphabet
}

// NewLayout builds the layout for id.
func NewLayout(id LayoutID) *Layout {
	switch id {
	case LayoutEntropy:
		return newEntropyLayout()
	default:
		return newASCIILayout()
	}
}

func (l *Layout) ID() LayoutID { return l.id }

// HintByte maps a group value to its wire byte.
func (l *Layout) HintByte(group byte) byte { return l.hint[group&0x3F] }

// GroupOf maps a wire byte back to its group. ok is false for padding bytes
// and for anything outside the alphabet.
//
// Note this is an EXACT membership test, deliberately stricter than upstream's
// `(b & 0x40) == 0x40` bitmask check, which also accepts 0x80..0xFF. Under the
// upstream rule any stray UTF-8 text that reached the decoder would be
// silently consumed as clues and corrupt the plaintext; here it is an error.
// See spec/CODEC.md §1.4.
func (l *Layout) GroupOf(wire byte) (byte, bool) {
	if !l.isHint[wire] {
		return 0, false
	}
	return l.group[wire], true
}

// IsHint reports whether the byte is in the clue alphabet.
func (l *Layout) IsHint(wire byte) bool { return l.isHint[wire] }

// IsPadding reports whether the byte is a filler byte.
func (l *Layout) IsPadding(wire byte) bool {
	for _, p := range l.padding {
		if p == wire {
			return true
		}
	}
	return false
}

// PaddingPool returns the filler alphabet.
func (l *Layout) PaddingPool() []byte { return l.padding }

func newASCIILayout() *Layout {
	l := &Layout{id: LayoutASCII}
	for g := 0; g < 64; g++ {
		b := byte(0x40 | g)
		if b == 0x7F {
			// 0x7F is DEL, not printable. The alphabet needs 64 symbols and
			// 0x40..0x7E only supplies 63, so the 64th borrows '\n'. Safe here
			// because the ciphertext's only transports are SQLite TEXT and
			// JSON, both of which carry '\n' exactly.
			b = '\n'
		}
		l.hint[g] = b
		l.isHint[b] = true
		l.group[b] = byte(g)
	}
	l.padding = make([]byte, 0, 32)
	for i := 0; i < 32; i++ {
		l.padding = append(l.padding, byte(0x20+i))
	}
	return l
}

func newEntropyLayout() *Layout {
	l := &Layout{id: LayoutEntropy}
	for g := 0; g < 64; g++ {
		// group = (digit<<4)|pos  ->  wire = (digit<<5)|pos
		b := ((byte(g) & 0x30) << 1) | (byte(g) & 0x0F)
		l.hint[g] = b
		l.isHint[b] = true
		l.group[b] = byte(g)
	}
	l.padding = make([]byte, 0, 16)
	for i := 0; i < 8; i++ {
		l.padding = append(l.padding, byte(0x80+i), byte(0x10+i))
	}
	return l
}

// ValidateLayout checks the invariants the codec relies on. Any violation
// would make decode ambiguous, so this is asserted at startup.
func (l *Layout) ValidateLayout() error {
	seen := map[byte]int{}
	for g := 0; g < 64; g++ {
		b := l.hint[g]
		if prev, dup := seen[b]; dup {
			return fmt.Errorf("layout %s: wire byte 0x%02x used by groups %d and %d", l.id, b, prev, g)
		}
		seen[b] = g
		if l.group[b] != byte(g) {
			return fmt.Errorf("layout %s: group round-trip failed for %d", l.id, g)
		}
	}
	for _, p := range l.padding {
		if l.isHint[p] {
			return fmt.Errorf("layout %s: padding byte 0x%02x is also a clue byte", l.id, p)
		}
	}
	return nil
}
