package services

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"qrforqa/internal/codec"
	"qrforqa/internal/models"
	"qrforqa/internal/schema"
)

// ImageView describes a stored image without exposing its ciphertext.
type ImageView struct {
	ID        uint   `json:"id"`
	FieldKey  string `json:"fieldKey"`
	Mime      string `json:"mime"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	SizeBytes int64  `json:"sizeBytes"`
	URL       string `json:"url"`
}

// ResponseView is a fully decoded response, which is what the administrator
// always sees.
type ResponseView struct {
	ID              uint           `json:"id"`
	QuestionnaireID uint           `json:"questionnaireId"`
	Answers         map[string]any `json:"answers"`
	Images          []ImageView    `json:"images"`
	DurationMs      int64          `json:"durationMs"`
	Index           int            `json:"index"`
	CreatedAt       time.Time      `json:"createdAt"`
	UpdatedAt       time.Time      `json:"updatedAt"`
}

// Submit stores a respondent's submission.
//
// The ciphertext is persisted exactly as received rather than being decoded and
// re-encoded: the requirement is that what lands in the database is the sudoku
// encoding the browser produced. Decoding happens only to validate.
func (s *Service) Submit(slug, cipher, layoutName string, durationMs int64, ipHash, ua string) (*models.Response, error) {
	q, err := s.GetBySlug(slug)
	if err != nil {
		return nil, err
	}
	if q.Status != models.StatusPublished {
		return nil, fmt.Errorf("%w: 该问卷当前不接受提交", ErrForbidden)
	}
	layout, err := codec.ParseLayoutID(layoutName)
	if err != nil {
		return nil, err
	}
	if layout != s.layout(q) {
		return nil, fmt.Errorf("layout mismatch: questionnaire uses %s", q.Layout)
	}
	if strings.TrimSpace(cipher) == "" {
		return nil, errors.New("提交内容为空")
	}

	answers, err := s.vault.DecodeJSONMap(q.ID, layout, cipher)
	if err != nil {
		return nil, fmt.Errorf("提交内容无法解码(可能是编码不一致):%w", err)
	}
	def, err := s.Definition(q)
	if err != nil {
		return nil, err
	}
	imageIDs, err := s.collectImageIDs(q.ID, answers)
	if err != nil {
		return nil, err
	}
	if err := schema.ValidateAnswers(def, schema.Answers(answers), imageIDs); err != nil {
		return nil, err
	}

	if len(ua) > 500 {
		ua = ua[:500]
	}
	if durationMs < 0 || durationMs > 24*3600*1000 {
		durationMs = 0
	}

	resp := &models.Response{
		QuestionnaireID: q.ID,
		PayloadCipher:   cipher,
		Layout:          layout.String(),
		DurationMs:      durationMs,
		IPHash:          ipHash,
		UserAgent:       ua,
	}
	if err := s.db.Create(resp).Error; err != nil {
		return nil, err
	}
	if err := s.attachImages(resp.ID, q.ID, imageIDs); err != nil {
		return nil, err
	}
	return resp, nil
}

// collectImageIDs finds every image reference in a decoded answer set.
func (s *Service) collectImageIDs(questionnaireID uint, answers map[string]any) (map[uint]bool, error) {
	ids := map[uint]bool{}
	for _, v := range answers {
		list, ok := v.([]any)
		if !ok {
			continue
		}
		for _, x := range list {
			n, ok := x.(float64)
			if !ok || n <= 0 || n != float64(int(n)) {
				continue
			}
			ids[uint(n)] = true
		}
	}
	if len(ids) == 0 {
		return ids, nil
	}
	// Verify every referenced image exists AND belongs to this questionnaire,
	// so a submission cannot attach someone else's upload.
	var found []uint
	if err := s.db.Model(&models.ResponseImage{}).
		Where("questionnaire_id = ? AND id IN ?", questionnaireID, keys(ids)).
		Pluck("id", &found).Error; err != nil {
		return nil, err
	}
	for _, id := range found {
		delete(ids, id)
	}
	if len(ids) > 0 {
		return nil, fmt.Errorf("提交引用了无效的图片:%v", keys(ids))
	}
	return mapKeys(found), nil
}

func (s *Service) attachImages(responseID, questionnaireID uint, ids map[uint]bool) error {
	if len(ids) == 0 {
		return nil
	}
	return s.db.Model(&models.ResponseImage{}).
		Where("id IN ? AND questionnaire_id = ?", keys(ids), questionnaireID).
		Update("response_id", responseID).Error
}

// UploadImage stores an uploaded image.
//
// The client sends the sudoku ciphertext of the base64 encoding; the server
// decodes it only to verify it is well-formed, then stores the ciphertext as
// received.
func (s *Service) UploadImage(q *models.Questionnaire, fieldKey, cipher, layoutName, mime string, width, height int) (*models.ResponseImage, error) {
	layout, err := codec.ParseLayoutID(layoutName)
	if err != nil {
		return nil, err
	}
	if layout != s.layout(q) {
		return nil, fmt.Errorf("layout mismatch")
	}
	if int64(len(cipher)) > s.cfg.MaxUploadCipherBytes {
		return nil, fmt.Errorf("图片编码后体积过大(%.1f MB,上限 %.1f MB)",
			float64(len(cipher))/1048576, float64(s.cfg.MaxUploadCipherBytes)/1048576)
	}

	plain, err := s.vault.DecodeBytes(q.ID, layout, cipher)
	if err != nil {
		return nil, fmt.Errorf("图片内容无法解码:%w", err)
	}
	raw, err := base64.StdEncoding.DecodeString(string(plain))
	if err != nil {
		// Tolerate URL-safe and unpadded variants from canvas exports.
		raw, err = base64.RawStdEncoding.DecodeString(strings.TrimRight(string(plain), "="))
		if err != nil {
			return nil, errors.New("图片内容不是合法的 base64")
		}
	}
	if len(raw) == 0 {
		return nil, errors.New("图片内容为空")
	}
	if mime == "" || !strings.HasPrefix(mime, "image/") {
		return nil, fmt.Errorf("不支持的图片类型:%s", mime)
	}
	if len(mime) > 64 {
		mime = mime[:64]
	}

	img := &models.ResponseImage{
		QuestionnaireID: q.ID,
		FieldKey:        truncate(fieldKey, 64),
		Mime:            mime,
		Layout:          layout.String(),
		Width:           width,
		Height:          height,
		SizeBytes:       int64(len(raw)),
		DataCipher:      cipher,
	}
	if err := s.db.Create(img).Error; err != nil {
		return nil, err
	}
	return img, nil
}

// ImageBytes decodes an image back to its raw bytes for admin display.
func (s *Service) ImageBytes(id uint) ([]byte, string, error) {
	var img models.ResponseImage
	if err := s.db.First(&img, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, "", ErrNotFound
		}
		return nil, "", err
	}
	layout, err := codec.ParseLayoutID(img.Layout)
	if err != nil {
		return nil, "", err
	}
	plain, err := s.vault.DecodeBytes(img.QuestionnaireID, layout, img.DataCipher)
	if err != nil {
		return nil, "", err
	}
	raw, err := base64.StdEncoding.DecodeString(string(plain))
	if err != nil {
		raw, err = base64.RawStdEncoding.DecodeString(strings.TrimRight(string(plain), "="))
		if err != nil {
			return nil, "", fmt.Errorf("decode stored image base64: %w", err)
		}
	}
	return raw, img.Mime, nil
}

// ListResponses returns a page of decoded responses.
func (s *Service) ListResponses(q *models.Questionnaire, page, size int, newestFirst bool) ([]ResponseView, int64, error) {
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 200 {
		size = 20
	}
	var total int64
	if err := s.db.Model(&models.Response{}).Where("questionnaire_id = ?", q.ID).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	order := "id asc"
	if newestFirst {
		order = "id desc"
	}
	var rows []models.Response
	if err := s.db.Where("questionnaire_id = ?", q.ID).
		Order(order).Offset((page - 1) * size).Limit(size).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	views, err := s.decorate(q, rows, (page-1)*size)
	if err != nil {
		return nil, 0, err
	}
	return views, total, nil
}

// AllResponses decodes every response, for statistics and export.
func (s *Service) AllResponses(q *models.Questionnaire) ([]ResponseView, error) {
	var rows []models.Response
	if err := s.db.Where("questionnaire_id = ?", q.ID).Order("id asc").Find(&rows).Error; err != nil {
		return nil, err
	}
	return s.decorate(q, rows, 0)
}

func (s *Service) decorate(q *models.Questionnaire, rows []models.Response, offset int) ([]ResponseView, error) {
	layout := s.layout(q)
	out := make([]ResponseView, 0, len(rows))
	for i := range rows {
		row := &rows[i]
		v := ResponseView{
			ID: row.ID, QuestionnaireID: row.QuestionnaireID,
			DurationMs: row.DurationMs, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
			Index: offset + i + 1,
			Answers: map[string]any{},
			Images:  []ImageView{},
		}
		// One undecodable row must not blank the whole page: the admin needs to
		// see that it is broken and why.
		if answers, err := s.vault.DecodeJSONMap(q.ID, layout, row.PayloadCipher); err == nil {
			v.Answers = answers
		} else {
			v.Answers = map[string]any{"__error": err.Error()}
		}
		out = append(out, v)
	}

	ids := make([]uint, 0, len(out))
	for i := range out {
		ids = append(ids, out[i].ID)
	}
	if len(ids) == 0 {
		return out, nil
	}
	var imgs []models.ResponseImage
	if err := s.db.Where("response_id IN ?", ids).Order("id asc").Find(&imgs).Error; err != nil {
		return nil, err
	}
	byResponse := map[uint][]ImageView{}
	for _, img := range imgs {
		if img.ResponseID == nil {
			continue
		}
		byResponse[*img.ResponseID] = append(byResponse[*img.ResponseID], ImageView{
			ID: img.ID, FieldKey: img.FieldKey, Mime: img.Mime,
			Width: img.Width, Height: img.Height, SizeBytes: img.SizeBytes,
			URL: fmt.Sprintf("/api/admin/images/%d", img.ID),
		})
	}
	for i := range out {
		if list, ok := byResponse[out[i].ID]; ok {
			out[i].Images = list
		}
	}
	return out, nil
}

// GetResponse returns one decoded response.
func (s *Service) GetResponse(q *models.Questionnaire, id uint) (*ResponseView, error) {
	var row models.Response
	if err := s.db.Where("id = ? AND questionnaire_id = ?", id, q.ID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	views, err := s.decorate(q, []models.Response{row}, 0)
	if err != nil {
		return nil, err
	}
	return &views[0], nil
}

// RawResponse exposes the stored ciphertext and an explanation of it, for the
// codec inspector.
type RawResponse struct {
	ID           uint              `json:"id"`
	Cipher       string            `json:"cipher"`
	Layout       string            `json:"layout"`
	Length       int               `json:"length"`
	Inspection   codec.DecodeReport `json:"inspection"`
}

func (s *Service) RawResponse(q *models.Questionnaire, id uint) (*RawResponse, error) {
	var row models.Response
	if err := s.db.Where("id = ? AND questionnaire_id = ?", id, q.ID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	layout := s.layout(q)
	rep, err := s.vault.Inspect(q.ID, layout, row.PayloadCipher)
	if err != nil {
		return nil, err
	}
	return &RawResponse{
		ID: row.ID, Cipher: row.PayloadCipher, Layout: row.Layout,
		Length: len(row.PayloadCipher), Inspection: rep,
	}, nil
}

// UpdateResponse re-encodes an edited answer set. The plaintext is never
// written to the database — only the new ciphertext.
func (s *Service) UpdateResponse(q *models.Questionnaire, id uint, answers schema.Answers) (*models.Response, error) {
	var row models.Response
	if err := s.db.Where("id = ? AND questionnaire_id = ?", id, q.ID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	def, err := s.Definition(q)
	if err != nil {
		return nil, err
	}
	imageIDs, err := s.collectImageIDs(q.ID, map[string]any(answers))
	if err != nil {
		// Images may legitimately have been attached to this response already;
		// fall back to accepting the ones that exist for this questionnaire.
		imageIDs, err = s.existingImageIDs(q.ID, answers)
		if err != nil {
			return nil, err
		}
	}
	if err := schema.ValidateAnswers(def, answers, imageIDs); err != nil {
		return nil, err
	}
	cipher, err := s.vault.EncodeJSON(q.ID, s.layout(q), map[string]any(answers))
	if err != nil {
		return nil, err
	}
	row.PayloadCipher = cipher
	if err := s.db.Model(&row).Update("payload_cipher", cipher).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *Service) existingImageIDs(questionnaireID uint, answers schema.Answers) (map[uint]bool, error) {
	out := map[uint]bool{}
	var ids []uint
	for _, v := range answers {
		list, ok := v.([]any)
		if !ok {
			continue
		}
		for _, x := range list {
			if n, ok := x.(float64); ok && n > 0 {
				ids = append(ids, uint(n))
			}
		}
	}
	if len(ids) == 0 {
		return out, nil
	}
	var found []uint
	if err := s.db.Model(&models.ResponseImage{}).
		Where("questionnaire_id = ? AND id IN ?", questionnaireID, ids).
		Pluck("id", &found).Error; err != nil {
		return nil, err
	}
	for _, id := range found {
		out[id] = true
	}
	return out, nil
}

// DeleteResponse removes a response and its images.
func (s *Service) DeleteResponse(q *models.Questionnaire, id uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("response_id = ?", id).Delete(&models.ResponseImage{}).Error; err != nil {
			return err
		}
		res := tx.Where("id = ? AND questionnaire_id = ?", id, q.ID).Delete(&models.Response{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// SweepOrphanImages removes uploads that were never attached to a response,
// which is what an abandoned form submission leaves behind.
func (s *Service) SweepOrphanImages(olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan)
	res := s.db.Where("response_id IS NULL AND created_at < ?", cutoff).Delete(&models.ResponseImage{})
	return res.RowsAffected, res.Error
}

func keys(m map[uint]bool) []uint {
	out := make([]uint, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func mapKeys(ids []uint) map[uint]bool {
	out := make(map[uint]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
