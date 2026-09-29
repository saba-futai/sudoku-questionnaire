package models

import "time"

// Questionnaire status values.
const (
	StatusDraft     = "draft"
	StatusPublished = "published"
	StatusClosed    = "closed"
)

// Admin is the single administrator account. The password is stored as a
// bcrypt hash; nothing else about it is recoverable from the database.
type Admin struct {
	ID           uint      `gorm:"primarykey" json:"id"`
	Username     string    `gorm:"uniqueIndex;size:64;not null" json:"username"`
	PasswordHash string    `gorm:"not null" json:"-"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// Questionnaire holds the form definition.
//
// SchemaCipher is the sudoku ciphertext of the definition JSON. The database
// therefore contains no plaintext questionnaire content at all: titles,
// questions and options are all unreadable without the per-questionnaire key.
type Questionnaire struct {
	ID           uint      `gorm:"primarykey" json:"id"`
	Slug         string    `gorm:"uniqueIndex;size:64;not null" json:"slug"`
	Status       string    `gorm:"index;size:16;not null;default:draft" json:"status"`
	Layout       string    `gorm:"size:16;not null;default:ascii" json:"layout"`
	SchemaCipher string    `gorm:"type:text;not null" json:"-"`
	Version      int       `gorm:"not null;default:1" json:"version"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`

	// Populated by the service layer, not persisted here.
	ResponseCount int64 `gorm:"-" json:"responseCount"`
}

// Response is one submitted answer set.
//
// PayloadCipher is the sudoku ciphertext of the answers JSON. Editing a
// response re-encodes it; the plaintext is never written to disk.
type Response struct {
	ID              uint      `gorm:"primarykey" json:"id"`
	QuestionnaireID uint      `gorm:"index:idx_resp_q_created,priority:1;not null" json:"questionnaireId"`
	PayloadCipher   string    `gorm:"type:text;not null" json:"-"`
	Layout          string    `gorm:"size:16;not null;default:ascii" json:"layout"`
	DurationMs      int64     `json:"durationMs"`
	IPHash          string    `gorm:"size:64" json:"-"`
	UserAgent       string    `gorm:"size:512" json:"-"`
	CreatedAt       time.Time `gorm:"index:idx_resp_q_created,priority:2" json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

// ResponseImage holds an uploaded image.
//
// DataCipher is the sudoku ciphertext of the image's base64 encoding, exactly
// as specified: base64 into SQLite, and that base64 is itself sudoku-encoded
// first. Nothing here is a decodable image without the key.
type ResponseImage struct {
	ID              uint      `gorm:"primarykey" json:"id"`
	QuestionnaireID uint      `gorm:"index;not null" json:"questionnaireId"`
	ResponseID      *uint     `gorm:"index" json:"responseId"`
	FieldKey        string    `gorm:"size:64;not null" json:"fieldKey"`
	Mime            string    `gorm:"size:64;not null" json:"mime"`
	Layout          string    `gorm:"size:16;not null;default:ascii" json:"layout"`
	Width           int       `json:"width"`
	Height          int       `json:"height"`
	SizeBytes       int64     `json:"sizeBytes"`
	DataCipher      string    `gorm:"type:text;not null" json:"-"`
	CreatedAt       time.Time `json:"createdAt"`
}

// AuditLog records administrator actions, which is what makes the "增删查改"
// surface reviewable after the fact.
type AuditLog struct {
	ID         uint      `gorm:"primarykey" json:"id"`
	Actor      string    `gorm:"size:64;not null" json:"actor"`
	Action     string    `gorm:"size:64;not null;index" json:"action"`
	TargetType string    `gorm:"size:32" json:"targetType"`
	TargetID   uint      `json:"targetId"`
	Detail     string    `gorm:"size:512" json:"detail"`
	CreatedAt  time.Time `gorm:"index" json:"createdAt"`
}

// All returns every model, for AutoMigrate.
func All() []any {
	return []any{
		&Admin{},
		&Questionnaire{},
		&Response{},
		&ResponseImage{},
		&AuditLog{},
	}
}
