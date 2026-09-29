package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"qrforqa/internal/codec"
	"qrforqa/internal/middleware"
	"qrforqa/internal/models"
)

// PublicList returns published questionnaires for the landing page.
//
// Titles come back decoded, because a browse list would otherwise force the
// browser to derive a table per row just to render a heading.
func (h *Handler) PublicList(c *gin.Context) {
	list, err := h.svc.List(true)
	if err != nil {
		fail(c, err)
		return
	}
	// The public projection deliberately omits response counts and internal
	// identifiers beyond the slug.
	type item struct {
		Slug          string `json:"slug"`
		Title         string `json:"title"`
		Description   string `json:"description"`
		StepCount     int    `json:"stepCount"`
		QuestionCount int    `json:"questionCount"`
	}
	out := make([]item, 0, len(list))
	for _, q := range list {
		out = append(out, item{
			Slug: q.Slug, Title: q.Title, Description: q.Description,
			StepCount: q.StepCount, QuestionCount: q.QuestionCount,
		})
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

// PublicGet returns a questionnaire's CIPHERTEXT plus the derived key, so the
// respondent's browser decodes the definition itself.
//
// The master key stays on the server; handing out only the per-questionnaire
// derived key means one compromised questionnaire cannot read another's data.
// No plaintext questionnaire content crosses this endpoint.
func (h *Handler) PublicGet(c *gin.Context) {
	q, err := h.svc.GetBySlug(c.Param("slug"))
	if err != nil {
		fail(c, err)
		return
	}
	if q.Status != models.StatusPublished {
		c.JSON(http.StatusForbidden, gin.H{"error": "该问卷暂未发布或已关闭"})
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
	c.JSON(http.StatusOK, gin.H{
		"slug":         q.Slug,
		"version":      q.Version,
		"layout":       q.Layout,
		"derivedKey":   h.svc.Vault().DerivedKey(q.ID),
		"fingerprint":  fingerprint,
		"schemaCipher": q.SchemaCipher,
	})
}

type submitRequest struct {
	Cipher     string `json:"cipher"`
	Layout     string `json:"layout"`
	DurationMs int64  `json:"durationMs"`
}

// PublicSubmit accepts a sudoku-encoded answer set.
func (h *Handler) PublicSubmit(c *gin.Context) {
	var req submitRequest
	if !bindJSON(c, &req) {
		return
	}
	ipHash := middleware.HashIP(c.ClientIP(), h.svc.Vault().DerivedKey(0))
	resp, err := h.svc.Submit(
		c.Param("slug"), req.Cipher, req.Layout, req.DurationMs,
		ipHash, c.GetHeader("User-Agent"),
	)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": resp.ID, "createdAt": resp.CreatedAt})
}

type uploadRequest struct {
	Cipher   string `json:"cipher"` // sudoku ciphertext of the base64 image
	Layout   string `json:"layout"`
	Mime     string `json:"mime"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	FieldKey string `json:"fieldKey"`
}

// PublicUpload stores an uploaded image.
//
// The browser has already base64-encoded AND sudoku-encoded it; the server
// decodes only to verify, then stores the ciphertext exactly as received.
func (h *Handler) PublicUpload(c *gin.Context) {
	q, err := h.svc.GetBySlug(c.Param("slug"))
	if err != nil {
		fail(c, err)
		return
	}
	if q.Status != models.StatusPublished {
		c.JSON(http.StatusForbidden, gin.H{"error": "该问卷当前不接受上传"})
		return
	}

	var req uploadRequest
	if !bindJSON(c, &req) {
		return
	}
	if strings.TrimSpace(req.Cipher) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "图片内容为空"})
		return
	}

	img, err := h.svc.UploadImage(q, req.FieldKey, req.Cipher, req.Layout, req.Mime, req.Width, req.Height)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": img.ID, "sizeBytes": img.SizeBytes})
}
