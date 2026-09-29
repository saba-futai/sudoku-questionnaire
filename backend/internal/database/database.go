package database

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"qrforqa/internal/models"
)

// Open connects to SQLite and applies migrations.
//
// The driver is glebarez/sqlite (modernc, pure Go) so the binary is fully
// static and cross-compiles from macOS to linux/amd64 without CGO — which is
// what lets the server run with no toolchain and no package downloads.
func Open(path string, verbose bool) (*gorm.DB, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return nil, fmt.Errorf("create data dir: %w", err)
		}
	}

	// WAL plus a busy timeout, and a single writer connection: one writer
	// removes the SQLITE_BUSY retry class entirely, and a questionnaire app
	// will not come close to saturating it.
	dsn := path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)" +
		"&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(1)"

	logLevel := logger.Warn
	if verbose {
		logLevel = logger.Info
	}
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger:                                   logger.Default.LogMode(logLevel),
		DisableForeignKeyConstraintWhenMigrating: false,
	})
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(time.Hour)

	if err := db.AutoMigrate(models.All()...); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return db, nil
}
