package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"qrforqa/internal/codec"
)

// Config is the server's runtime configuration, read from the environment so
// that secrets live in a systemd EnvironmentFile rather than in the repository.
type Config struct {
	Addr    string // primary listener; everything is served here
	AltAddr string // optional secondary listener (empty disables it)

	DataDir string
	DBPath  string

	// MasterKey seeds every per-questionnaire sudoku table. It never leaves the
	// server: browsers receive only the derived per-questionnaire key.
	MasterKey string

	AdminUser     string
	AdminPassword string

	JWTSecret string
	TokenTTL  time.Duration

	Layout codec.LayoutID

	// MaxUploadCipherBytes caps a single image ciphertext. The ciphertext is
	// ~4x the base64 and ~5.33x the raw image, so this is roughly a 280KB JPEG.
	MaxUploadCipherBytes int64

	Dev bool
}

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envBool(key string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	return v == "1" || v == "true" || v == "yes"
}

// Load reads configuration from the environment.
//
// The master key and admin password are required. There is deliberately no
// silent fallback: an auto-generated key would make every restart render all
// stored ciphertext undecryptable, and a default password would be worse.
func Load() (*Config, error) {
	dataDir := env("QRFORQA_DATA_DIR", "./data")
	layout, err := codec.ParseLayoutID(env("QRFORQA_LAYOUT", "ascii"))
	if err != nil {
		return nil, err
	}

	ttl := 12 * time.Hour
	if v := os.Getenv("QRFORQA_TOKEN_TTL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			ttl = d
		} else {
			return nil, fmt.Errorf("QRFORQA_TOKEN_TTL: %w", err)
		}
	}

	c := &Config{
		Addr:                 env("QRFORQA_ADDR", ":8080"),
		AltAddr:              env("QRFORQA_ALT_ADDR", ""),
		DataDir:              dataDir,
		DBPath:               env("QRFORQA_DB", filepath.Join(dataDir, "qrforqa.db")),
		MasterKey:            os.Getenv("QRFORQA_MASTER_KEY"),
		AdminUser:            env("QRFORQA_ADMIN_USER", "admin"),
		AdminPassword:        os.Getenv("QRFORQA_ADMIN_PASSWORD"),
		JWTSecret:            os.Getenv("QRFORQA_JWT_SECRET"),
		TokenTTL:             ttl,
		Layout:               layout,
		MaxUploadCipherBytes: int64(envInt("QRFORQA_MAX_UPLOAD_BYTES", 2*1024*1024)),
		Dev:                  envBool("QRFORQA_DEV"),
	}

	// The entropy layout emits control bytes and bytes >= 0x80. Ciphertext is
	// stored and transported as a Go/JSON string, and encoding/json rewrites
	// invalid UTF-8 to U+FFFD — so an entropy-layout questionnaire would be
	// silently corrupted on the way out of the database. Refuse to start rather
	// than corrupt data later.
	if layout != codec.LayoutASCII {
		return nil, fmt.Errorf(
			"QRFORQA_LAYOUT=%s is not supported by this application: the storage and JSON transport "+
				"layers require the ASCII layout. The entropy layout is available in the codec "+
				"library and in the codec inspector only", layout)
	}

	if c.MasterKey == "" {
		return nil, fmt.Errorf("QRFORQA_MASTER_KEY is required (generate one with `qrforqa keygen`)")
	}
	if len(c.MasterKey) < 16 {
		return nil, fmt.Errorf("QRFORQA_MASTER_KEY must be at least 16 characters")
	}
	if c.JWTSecret == "" {
		// Deriving from the master key keeps the deployment to a single secret
		// while still giving the JWT its own key material.
		c.JWTSecret = "jwt:" + c.MasterKey
	}
	return c, nil
}

// IsFirstRun reports whether the database still needs its admin seeded.
func (c *Config) FirstRunCredentialsOK() error {
	if c.AdminPassword == "" {
		return fmt.Errorf("QRFORQA_ADMIN_PASSWORD is required on first run")
	}
	if len(c.AdminPassword) < 8 {
		return fmt.Errorf("QRFORQA_ADMIN_PASSWORD must be at least 8 characters")
	}
	return nil
}

// GenerateSecret returns a fresh 32-byte hex secret for deployment tooling.
func GenerateSecret() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("config: crypto/rand unavailable: " + err.Error())
	}
	return hex.EncodeToString(b[:])
}
