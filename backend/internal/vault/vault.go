// Package vault is the single boundary between plaintext and sudoku-encoded
// storage.
//
// Everything that touches the database goes through here, so the encoding rule
// is enforced in one place rather than at every call site.
package vault

import (
	"encoding/json"
	"fmt"
	"strconv"
	"sync"

	"qrforqa/internal/codec"
)

// Vault derives and caches per-questionnaire sudoku tables.
//
// The master key never leaves the process. Browsers receive only the derived
// per-questionnaire key, so one compromised questionnaire does not expose any
// other questionnaire's data.
type Vault struct {
	masterKey string
	validity  *codec.Validity

	mu     sync.RWMutex
	tables map[string]*codec.Table
}

// New builds a vault over the shipped validity bitmap.
func New(masterKey string) (*Vault, error) {
	v, err := codec.EmbeddedValidity()
	if err != nil {
		return nil, fmt.Errorf("vault: %w", err)
	}
	return &Vault{
		masterKey: masterKey,
		validity:  v,
		tables:    make(map[string]*codec.Table),
	}, nil
}

// DeriveKey returns the per-questionnaire key. MUST match deriveKey() in
// frontend/src/codec/index.ts exactly, or the browser and server derive
// different tables.
func DeriveKey(masterKey string, questionnaireID uint) string {
	return masterKey + "|q:" + strconv.FormatUint(uint64(questionnaireID), 10)
}

// Table returns the cached table for a questionnaire, building it on first use.
// A build is a few milliseconds, so a questionnaire pays it once per process.
func (v *Vault) Table(questionnaireID uint, layout codec.LayoutID) (*codec.Table, error) {
	cacheKey := strconv.FormatUint(uint64(questionnaireID), 10) + "|" + layout.String()

	v.mu.RLock()
	t, ok := v.tables[cacheKey]
	v.mu.RUnlock()
	if ok {
		return t, nil
	}

	v.mu.Lock()
	defer v.mu.Unlock()
	if t, ok := v.tables[cacheKey]; ok {
		return t, nil
	}
	t, err := codec.BuildTable(DeriveKey(v.masterKey, questionnaireID), layout, v.validity)
	if err != nil {
		return nil, err
	}
	v.tables[cacheKey] = t
	return t, nil
}

// DerivedKey exposes the browser-facing key for a questionnaire.
func (v *Vault) DerivedKey(questionnaireID uint) string {
	return DeriveKey(v.masterKey, questionnaireID)
}

// Fingerprint identifies the questionnaire's table, letting the browser verify
// its locally derived table matches the server's before it sends anything.
func (v *Vault) Fingerprint(questionnaireID uint, layout codec.LayoutID) (string, error) {
	t, err := v.Table(questionnaireID, layout)
	if err != nil {
		return "", err
	}
	return t.Fingerprint(), nil
}

// EncodeBytes encodes raw bytes and returns the ciphertext as text.
func (v *Vault) EncodeBytes(questionnaireID uint, layout codec.LayoutID, plain []byte) (string, error) {
	t, err := v.Table(questionnaireID, layout)
	if err != nil {
		return "", err
	}
	return string(codec.Encode(t, codec.NewRandomPRNG(), plain, codec.MaxPadDefault)), nil
}

// DecodeBytes decodes ciphertext back to raw bytes.
func (v *Vault) DecodeBytes(questionnaireID uint, layout codec.LayoutID, cipher string) ([]byte, error) {
	t, err := v.Table(questionnaireID, layout)
	if err != nil {
		return nil, err
	}
	plain, err := codec.Decode(t, []byte(cipher))
	if err != nil {
		return nil, fmt.Errorf("decode questionnaire %d: %w", questionnaireID, err)
	}
	return plain, nil
}

// EncodeJSON encodes any JSON-serialisable value.
func (v *Vault) EncodeJSON(questionnaireID uint, layout codec.LayoutID, value any) (string, error) {
	plain, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("marshal: %w", err)
	}
	return v.EncodeBytes(questionnaireID, layout, plain)
}

// DecodeJSON decodes ciphertext into out.
func (v *Vault) DecodeJSON(questionnaireID uint, layout codec.LayoutID, cipher string, out any) error {
	plain, err := v.DecodeBytes(questionnaireID, layout, cipher)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(plain, out); err != nil {
		return fmt.Errorf("unmarshal questionnaire %d payload: %w", questionnaireID, err)
	}
	return nil
}

// DecodeJSONMap decodes into a generic map, for answer payloads whose shape
// depends on the questionnaire definition.
func (v *Vault) DecodeJSONMap(questionnaireID uint, layout codec.LayoutID, cipher string) (map[string]any, error) {
	out := map[string]any{}
	if err := v.DecodeJSON(questionnaireID, layout, cipher, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Inspect explains a ciphertext without failing, backing the codec inspector.
func (v *Vault) Inspect(questionnaireID uint, layout codec.LayoutID, cipher string) (codec.DecodeReport, error) {
	t, err := v.Table(questionnaireID, layout)
	if err != nil {
		return codec.DecodeReport{}, err
	}
	return codec.Inspect(t, []byte(cipher)), nil
}

// EncodingStats reports how many distinct puzzles the questionnaire's table
// offers per byte value, for the inspector's "why is this ciphertext different
// every time" panel.
func (v *Vault) EncodingStats(questionnaireID uint, layout codec.LayoutID) (minCount, maxCount, distinctPuzzles int, err error) {
	t, err := v.Table(questionnaireID, layout)
	if err != nil {
		return 0, 0, 0, err
	}
	counts := t.EncodeCounts()
	minCount = counts[0]
	maxCount = counts[0]
	for _, c := range counts {
		if c < minCount {
			minCount = c
		}
		if c > maxCount {
			maxCount = c
		}
	}
	return minCount, maxCount, t.DecodeMapSize(), nil
}
