package handlers

import (
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"qrforqa/internal/codec"
	"qrforqa/internal/middleware"
	"qrforqa/internal/models"
	"qrforqa/internal/schema"
	"qrforqa/internal/services"
)

// ------------------------------------------------------------------ auth ----

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *Handler) Login(c *gin.Context) {
	var req loginRequest
	if !bindJSON(c, &req) {
		return
	}
	admin, err := h.svc.Authenticate(strings.TrimSpace(req.Username), req.Password)
	if err != nil {
		h.svc.Audit(req.Username, "login.failed", "admin", 0, c.ClientIP())
		fail(c, err)
		return
	}
	token, expires, err := h.svc.IssueToken(admin)
	if err != nil {
		fail(c, err)
		return
	}
	h.svc.Audit(admin.Username, "login.success", "admin", admin.ID, c.ClientIP())
	c.JSON(http.StatusOK, gin.H{
		"token": token, "username": admin.Username, "expiresAt": expires,
	})
}

func (h *Handler) Me(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"id":       middleware.AdminID(c),
		"username": middleware.AdminUser(c),
	})
}

type passwordRequest struct {
	Current string `json:"current"`
	Next    string `json:"next"`
}

func (h *Handler) ChangePassword(c *gin.Context) {
	var req passwordRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := h.svc.ChangePassword(middleware.AdminID(c), req.Current, req.Next); err != nil {
		fail(c, err)
		return
	}
	h.svc.Audit(middleware.AdminUser(c), "password.change", "admin", middleware.AdminID(c), "")
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// -------------------------------------------------------------- dashboard ---

func (h *Handler) Overview(c *gin.Context) {
	counts, err := h.svc.Overview()
	if err != nil {
		fail(c, err)
		return
	}
	board, err := h.svc.Leaderboard(5)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"counts": counts, "leaderboard": board})
}

func (h *Handler) Leaderboard(c *gin.Context) {
	board, err := h.svc.Leaderboard(queryInt(c, "limit", 5))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": board})
}

func (h *Handler) Audit(c *gin.Context) {
	logs, err := h.svc.RecentAudit(queryInt(c, "limit", 50))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": logs})
}

// ---------------------------------------------------------- questionnaires ---

func (h *Handler) AdminListQuestionnaires(c *gin.Context) {
	list, err := h.svc.List(false)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": list})
}

func (h *Handler) AdminCreateQuestionnaire(c *gin.Context) {
	var in services.QuestionnaireInput
	if !bindJSON(c, &in) {
		return
	}
	q, err := h.svc.Create(in)
	if err != nil {
		fail(c, err)
		return
	}
	h.svc.Audit(middleware.AdminUser(c), "questionnaire.create", "questionnaire", q.ID, in.Definition.Title)
	c.JSON(http.StatusCreated, h.questionnaireDetail(q))
}

// AdminGetQuestionnaire returns the DECODED definition, which is what the
// administrator always sees.
func (h *Handler) AdminGetQuestionnaire(c *gin.Context) {
	id, ok := paramUint(c, "id")
	if !ok {
		return
	}
	q, err := h.svc.Get(id)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, h.questionnaireDetail(q))
}

// questionnaireDetail renders a questionnaire with its definition DECODED.
//
// If the ciphertext cannot be decoded — key rotated, row edited by hand — the
// response still describes the row, so the admin can see and repair it rather
// than facing a bare 500.
func (h *Handler) questionnaireDetail(q *models.Questionnaire) gin.H {
	out := gin.H{
		"id": q.ID, "slug": q.Slug, "status": q.Status,
		"version": q.Version, "layout": q.Layout,
		"createdAt": q.CreatedAt, "updatedAt": q.UpdatedAt,
	}
	def, err := h.svc.Definition(q)
	if err != nil {
		out["undecodable"] = true
		out["error"] = "问卷定义无法解码:" + err.Error()
		return out
	}
	out["definition"] = def
	return out
}

func (h *Handler) AdminUpdateQuestionnaire(c *gin.Context) {
	id, ok := paramUint(c, "id")
	if !ok {
		return
	}
	var in services.QuestionnaireInput
	if !bindJSON(c, &in) {
		return
	}
	q, err := h.svc.Update(id, in)
	if err != nil {
		fail(c, err)
		return
	}
	h.svc.Audit(middleware.AdminUser(c), "questionnaire.update", "questionnaire", q.ID, in.Definition.Title)
	c.JSON(http.StatusOK, gin.H{"id": q.ID, "version": q.Version, "status": q.Status})
}

func (h *Handler) AdminDeleteQuestionnaire(c *gin.Context) {
	id, ok := paramUint(c, "id")
	if !ok {
		return
	}
	q, err := h.svc.Get(id)
	if err != nil {
		fail(c, err)
		return
	}
	if err := h.svc.Delete(id); err != nil {
		fail(c, err)
		return
	}
	h.svc.Audit(middleware.AdminUser(c), "questionnaire.delete", "questionnaire", id, q.Slug)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type statusRequest struct {
	Status string `json:"status"`
}

func (h *Handler) AdminSetStatus(c *gin.Context) {
	id, ok := paramUint(c, "id")
	if !ok {
		return
	}
	var req statusRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := h.svc.SetStatus(id, req.Status); err != nil {
		fail(c, err)
		return
	}
	h.svc.Audit(middleware.AdminUser(c), "questionnaire.status", "questionnaire", id, req.Status)
	c.JSON(http.StatusOK, gin.H{"ok": true, "status": req.Status})
}

// -------------------------------------------------------------- responses ---

func (h *Handler) AdminListResponses(c *gin.Context) {
	id, ok := paramUint(c, "id")
	if !ok {
		return
	}
	q, err := h.svc.Get(id)
	if err != nil {
		fail(c, err)
		return
	}
	page := queryInt(c, "page", 1)
	size := queryInt(c, "size", 20)
	views, total, err := h.svc.ListResponses(q, page, size, c.Query("order") != "asc")
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"items": views, "total": total, "page": page, "size": size,
	})
}

func (h *Handler) AdminGetResponse(c *gin.Context) {
	q, rid, ok := h.qAndResponse(c)
	if !ok {
		return
	}
	view, err := h.svc.GetResponse(q, rid)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, view)
}

type updateResponseRequest struct {
	Answers map[string]any `json:"answers"`
}

// AdminUpdateResponse re-encodes an edited answer set. The plaintext is never
// written to disk.
func (h *Handler) AdminUpdateResponse(c *gin.Context) {
	q, rid, ok := h.qAndResponse(c)
	if !ok {
		return
	}
	var req updateResponseRequest
	if !bindJSON(c, &req) {
		return
	}
	if _, err := h.svc.UpdateResponse(q, rid, schema.Answers(req.Answers)); err != nil {
		fail(c, err)
		return
	}
	h.svc.Audit(middleware.AdminUser(c), "response.update", "response", rid, q.Slug)
	view, err := h.svc.GetResponse(q, rid)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, view)
}

func (h *Handler) AdminDeleteResponse(c *gin.Context) {
	q, rid, ok := h.qAndResponse(c)
	if !ok {
		return
	}
	if err := h.svc.DeleteResponse(q, rid); err != nil {
		fail(c, err)
		return
	}
	h.svc.Audit(middleware.AdminUser(c), "response.delete", "response", rid, q.Slug)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// AdminRawResponse returns the stored ciphertext so the inspector can show the
// administrator exactly what is on disk next to the decoded values.
func (h *Handler) AdminRawResponse(c *gin.Context) {
	q, rid, ok := h.qAndResponse(c)
	if !ok {
		return
	}
	raw, err := h.svc.RawResponse(q, rid)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, raw)
}

func (h *Handler) qAndResponse(c *gin.Context) (*models.Questionnaire, uint, bool) {
	id, ok := paramUint(c, "id")
	if !ok {
		return nil, 0, false
	}
	rid, ok := paramUint(c, "rid")
	if !ok {
		return nil, 0, false
	}
	q, err := h.svc.Get(id)
	if err != nil {
		fail(c, err)
		return nil, 0, false
	}
	return q, rid, true
}

// ----------------------------------------------------------------- stats ----

func (h *Handler) AdminStats(c *gin.Context) {
	id, ok := paramUint(c, "id")
	if !ok {
		return
	}
	q, err := h.svc.Get(id)
	if err != nil {
		fail(c, err)
		return
	}
	stats, err := h.svc.Stats(q)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, stats)
}

// ---------------------------------------------------------------- export ----

// AdminExport streams DECODED content: CSV, XLSX or JSON.
func (h *Handler) AdminExport(c *gin.Context) {
	id, ok := paramUint(c, "id")
	if !ok {
		return
	}
	q, err := h.svc.Get(id)
	if err != nil {
		fail(c, err)
		return
	}
	format, err := services.ParseExportFormat(c.Query("format"))
	if err != nil {
		fail(c, err)
		return
	}
	data, contentType, filename, err := h.svc.Export(q, format)
	if err != nil {
		fail(c, err)
		return
	}
	h.svc.Audit(middleware.AdminUser(c), "questionnaire.export", "questionnaire", id, string(format))

	// RFC 5987 so non-ASCII filenames survive.
	asciiName := "export." + path.Ext(filename)
	c.Header("Content-Disposition",
		`attachment; filename="`+asciiName+`"; filename*=UTF-8''`+url.PathEscape(filename))
	c.Header("Content-Type", contentType)
	c.Header("Content-Length", strconv.Itoa(len(data)))
	c.Data(http.StatusOK, contentType, data)
}

// ---------------------------------------------------------------- images ----

// AdminImage decodes a stored image and serves the real bytes.
func (h *Handler) AdminImage(c *gin.Context) {
	id, ok := paramUint(c, "id")
	if !ok {
		return
	}
	raw, mime, err := h.svc.ImageBytes(id)
	if err != nil {
		fail(c, err)
		return
	}
	c.Header("Cache-Control", "private, max-age=300")
	c.Data(http.StatusOK, mime, raw)
}

// ----------------------------------------------------------------- codec ----

// AdminCodecInfo reports the table properties of a questionnaire, so the
// inspector can explain what the administrator is looking at.
func (h *Handler) AdminCodecInfo(c *gin.Context) {
	id, ok := paramUint(c, "id")
	if !ok {
		return
	}
	q, err := h.svc.Get(id)
	if err != nil {
		fail(c, err)
		return
	}
	layout, err := codec.ParseLayoutID(q.Layout)
	if err != nil {
		fail(c, err)
		return
	}
	fingerprint, err := h.svc.Vault().Fingerprint(q.ID, layout)
	if err != nil {
		fail(c, err)
		return
	}
	minC, maxC, distinct, err := h.svc.Vault().EncodingStats(q.ID, layout)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"questionnaireId": q.ID,
		"slug":            q.Slug,
		"layout":          q.Layout,
		"fingerprint":     fingerprint,
		"derivedKey":      h.svc.Vault().DerivedKey(q.ID),
		"minEncodings":    minC,
		"maxEncodings":    maxC,
		"distinctPuzzles": distinct,
		"gridCount":       codec.GridCount,
		"posSetCount":     codec.PosSetCount,
		"gridsSha256":     codec.GridsSHA256(),
	})
}

type codecEncodeRequest struct {
	QuestionnaireID uint   `json:"questionnaireId"`
	Text            string `json:"text"`
	Layout          string `json:"layout"`
}

// CodecEncode encodes arbitrary text, for the inspector's live demo.
func (h *Handler) CodecEncode(c *gin.Context) {
	var req codecEncodeRequest
	if !bindJSON(c, &req) {
		return
	}
	if len(req.Text) > 200_000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "演示文本过长"})
		return
	}
	layout, err := codec.ParseLayoutID(req.Layout)
	if err != nil {
		fail(c, err)
		return
	}
	cipher, err := h.svc.Vault().EncodeBytes(req.QuestionnaireID, layout, []byte(req.Text))
	if err != nil {
		fail(c, err)
		return
	}
	rep, _ := h.svc.Vault().Inspect(req.QuestionnaireID, layout, cipher)
	c.JSON(http.StatusOK, gin.H{
		"cipher":       cipher,
		"length":       len(cipher),
		"plaintextLen": len(req.Text),
		"expansion":    expansion(len(req.Text), len(cipher)),
		"inspection":   rep,
	})
}

type codecDecodeRequest struct {
	QuestionnaireID uint   `json:"questionnaireId"`
	Cipher          string `json:"cipher"`
	Layout          string `json:"layout"`
}

// CodecDecode decodes arbitrary ciphertext, for the inspector.
func (h *Handler) CodecDecode(c *gin.Context) {
	var req codecDecodeRequest
	if !bindJSON(c, &req) {
		return
	}
	layout, err := codec.ParseLayoutID(req.Layout)
	if err != nil {
		fail(c, err)
		return
	}
	plain, err := h.svc.Vault().DecodeBytes(req.QuestionnaireID, layout, req.Cipher)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"text":   string(plain),
		"length": len(plain),
	})
}

func expansion(plainLen, cipherLen int) float64 {
	if plainLen == 0 {
		return 0
	}
	return float64(cipherLen) / float64(plainLen)
}
