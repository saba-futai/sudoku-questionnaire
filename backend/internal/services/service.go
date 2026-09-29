// Package services holds the application logic. Every read and write of
// questionnaire content passes through the vault, so plaintext never reaches
// the database.
package services

import (
	"crypto/rand"
	"encoding/base32"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"gorm.io/gorm"

	"qrforqa/internal/codec"
	"qrforqa/internal/config"
	"qrforqa/internal/models"
	"qrforqa/internal/schema"
	"qrforqa/internal/vault"
)

var (
	ErrNotFound  = errors.New("not found")
	ErrConflict  = errors.New("conflict")
	ErrForbidden = errors.New("forbidden")
)

type Service struct {
	db    *gorm.DB
	vault *vault.Vault
	cfg   *config.Config
}

func New(db *gorm.DB, v *vault.Vault, cfg *config.Config) *Service {
	return &Service{db: db, vault: v, cfg: cfg}
}

func (s *Service) Vault() *vault.Vault { return s.vault }
func (s *Service) DB() *gorm.DB        { return s.db }

func (s *Service) layout(q *models.Questionnaire) codec.LayoutID {
	id, err := codec.ParseLayoutID(q.Layout)
	if err != nil {
		return codec.LayoutASCII
	}
	return id
}

// Audit records an administrator action.
func (s *Service) Audit(actor, action, targetType string, targetID uint, detail string) {
	if len(detail) > 500 {
		detail = detail[:500]
	}
	_ = s.db.Create(&models.AuditLog{
		Actor: actor, Action: action,
		TargetType: targetType, TargetID: targetID, Detail: detail,
	}).Error
}

// ---------------------------------------------------------------- slugs ----

const slugAlphabet = "abcdefghijkmnpqrstuvwxyz23456789"

var (
	slugEncoding = base32.NewEncoding(slugAlphabet).WithPadding(base32.NoPadding)
	slugRe       = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,63}$`)
)

func newSlug() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("services: crypto/rand unavailable: " + err.Error())
	}
	return strings.ToLower(slugEncoding.EncodeToString(b[:])[:10])
}

// ---------------------------------------------------- questionnaire read ----

// QuestionnaireSummary is the list/browse projection. Title and description are
// decoded server-side so that browsing does not require the browser to build a
// table for every row.
type QuestionnaireSummary struct {
	ID            uint      `json:"id"`
	Slug          string    `json:"slug"`
	Status        string    `json:"status"`
	Version       int       `json:"version"`
	Layout        string    `json:"layout"`
	Title         string    `json:"title"`
	Description   string    `json:"description"`
	StepCount     int       `json:"stepCount"`
	QuestionCount int       `json:"questionCount"`
	ResponseCount int64     `json:"responseCount"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

// List returns questionnaire summaries. When publishedOnly is set, drafts and
// closed questionnaires are omitted, for the public landing page.
func (s *Service) List(publishedOnly bool) ([]QuestionnaireSummary, error) {
	var rows []models.Questionnaire
	q := s.db.Order("id desc")
	if publishedOnly {
		q = q.Where("status = ?", models.StatusPublished)
	}
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}

	out := make([]QuestionnaireSummary, 0, len(rows))
	for i := range rows {
		row := &rows[i]
		sum := QuestionnaireSummary{
			ID: row.ID, Slug: row.Slug, Status: row.Status, Version: row.Version,
			Layout: row.Layout, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		}
		// A questionnaire whose ciphertext cannot be decoded (key rotated, row
		// hand-edited) must not break the whole listing; surface it as
		// unreadable instead and let the admin see the problem.
		if def, err := s.Definition(row); err == nil {
			sum.Title = def.Title
			sum.Description = def.Description
			sum.StepCount = len(def.Steps)
			sum.QuestionCount = len(def.AllQuestions())
		} else {
			sum.Title = "⚠ 无法解码"
			sum.Description = err.Error()
		}
		out = append(out, sum)
	}

	// Counts in one grouped query rather than per row.
	type countRow struct {
		QuestionnaireID uint
		N               int64
	}
	var counts []countRow
	if err := s.db.Model(&models.Response{}).
		Select("questionnaire_id, count(*) as n").
		Group("questionnaire_id").Scan(&counts).Error; err != nil {
		return nil, err
	}
	byID := make(map[uint]int64, len(counts))
	for _, c := range counts {
		byID[c.QuestionnaireID] = c.N
	}
	for i := range out {
		out[i].ResponseCount = byID[out[i].ID]
	}
	return out, nil
}

func (s *Service) Get(id uint) (*models.Questionnaire, error) {
	var q models.Questionnaire
	if err := s.db.First(&q, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &q, nil
}

func (s *Service) GetBySlug(slug string) (*models.Questionnaire, error) {
	var q models.Questionnaire
	if err := s.db.Where("slug = ?", slug).First(&q).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &q, nil
}

// Definition decodes a questionnaire's stored definition.
func (s *Service) Definition(q *models.Questionnaire) (*schema.Definition, error) {
	var def schema.Definition
	if err := s.vault.DecodeJSON(q.ID, s.layout(q), q.SchemaCipher, &def); err != nil {
		return nil, err
	}
	if def.Title == "" {
		def.Title = "未命名问卷"
	}
	return &def, nil
}

// --------------------------------------------------- questionnaire write ----

type QuestionnaireInput struct {
	Slug       string             `json:"slug"`
	Status     string             `json:"status"`
	Definition schema.Definition  `json:"definition"`
}

func validStatus(s string) bool {
	return s == models.StatusDraft || s == models.StatusPublished || s == models.StatusClosed
}

// Create validates a definition, encodes it, and stores only the ciphertext.
func (s *Service) Create(in QuestionnaireInput) (*models.Questionnaire, error) {
	if err := in.Definition.Validate(); err != nil {
		return nil, err
	}
	status := in.Status
	if status == "" {
		status = models.StatusDraft
	}
	if !validStatus(status) {
		return nil, fmt.Errorf("未知状态:%s", status)
	}
	slug := strings.TrimSpace(in.Slug)
	if slug == "" {
		slug = newSlug()
	} else if !slugRe.MatchString(slug) {
		return nil, errors.New("自定义链接只能包含小写字母、数字和连字符(2-64 位)")
	}

	var existing int64
	if err := s.db.Model(&models.Questionnaire{}).Where("slug = ?", slug).Count(&existing).Error; err != nil {
		return nil, err
	}
	if existing > 0 {
		return nil, fmt.Errorf("%w: 链接 %q 已被占用", ErrConflict, slug)
	}

	q := &models.Questionnaire{
		Slug: slug, Status: status, Layout: s.cfg.Layout.String(), Version: 1,
	}
	// The ID is needed before encoding, because the table is derived from it.
	if err := s.db.Create(q).Error; err != nil {
		return nil, err
	}
	cipher, err := s.vault.EncodeJSON(q.ID, s.cfg.Layout, in.Definition)
	if err != nil {
		_ = s.db.Delete(q).Error
		return nil, err
	}
	q.SchemaCipher = cipher
	if err := s.db.Model(q).Update("schema_cipher", cipher).Error; err != nil {
		return nil, err
	}
	return q, nil
}

// Update re-encodes the definition. The questionnaire ID — and therefore the
// sudoku table — is unchanged, so existing responses stay readable.
func (s *Service) Update(id uint, in QuestionnaireInput) (*models.Questionnaire, error) {
	q, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	if err := in.Definition.Validate(); err != nil {
		return nil, err
	}
	cipher, err := s.vault.EncodeJSON(q.ID, s.layout(q), in.Definition)
	if err != nil {
		return nil, err
	}

	q.SchemaCipher = cipher
	q.Version++
	if in.Status != "" {
		if !validStatus(in.Status) {
			return nil, fmt.Errorf("未知状态:%s", in.Status)
		}
		q.Status = in.Status
	}
	if err := s.db.Model(q).Updates(map[string]any{
		"schema_cipher": cipher, "version": q.Version, "status": q.Status,
	}).Error; err != nil {
		return nil, err
	}
	return q, nil
}

func (s *Service) SetStatus(id uint, status string) error {
	if !validStatus(status) {
		return fmt.Errorf("未知状态:%s", status)
	}
	res := s.db.Model(&models.Questionnaire{}).Where("id = ?", id).Update("status", status)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete removes a questionnaire along with its responses and images.
func (s *Service) Delete(id uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("questionnaire_id = ?", id).Delete(&models.ResponseImage{}).Error; err != nil {
			return err
		}
		if err := tx.Where("questionnaire_id = ?", id).Delete(&models.Response{}).Error; err != nil {
			return err
		}
		res := tx.Delete(&models.Questionnaire{}, id)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// OverviewCounts powers the admin dashboard.
type OverviewCounts struct {
	Questionnaires int64 `json:"questionnaires"`
	Published      int64 `json:"published"`
	Draft          int64 `json:"draft"`
	Closed         int64 `json:"closed"`
	Responses      int64 `json:"responses"`
	Images         int64 `json:"images"`
	ResponsesToday int64 `json:"responsesToday"`
}

func (s *Service) Overview() (*OverviewCounts, error) {
	var out OverviewCounts
	db := s.db
	if err := db.Model(&models.Questionnaire{}).Count(&out.Questionnaires).Error; err != nil {
		return nil, err
	}
	for _, pair := range []struct {
		status string
		dst    *int64
	}{
		{models.StatusPublished, &out.Published},
		{models.StatusDraft, &out.Draft},
		{models.StatusClosed, &out.Closed},
	} {
		if err := db.Model(&models.Questionnaire{}).Where("status = ?", pair.status).Count(pair.dst).Error; err != nil {
			return nil, err
		}
	}
	if err := db.Model(&models.Response{}).Count(&out.Responses).Error; err != nil {
		return nil, err
	}
	if err := db.Model(&models.ResponseImage{}).Count(&out.Images).Error; err != nil {
		return nil, err
	}
	start := time.Now().Truncate(24 * time.Hour)
	if err := db.Model(&models.Response{}).Where("created_at >= ?", start).Count(&out.ResponsesToday).Error; err != nil {
		return nil, err
	}
	return &out, nil
}

// RecentAudit returns the newest audit entries.
func (s *Service) RecentAudit(limit int) ([]models.AuditLog, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var out []models.AuditLog
	err := s.db.Order("id desc").Limit(limit).Find(&out).Error
	return out, err
}
