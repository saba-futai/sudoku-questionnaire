// Package handlers wires the HTTP surface.
//
// Two audiences: /api/public is unauthenticated (respondents fill forms) and
// /api/admin is JWT-protected (the administrator sees everything decoded).
package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"qrforqa/internal/codec"
	"qrforqa/internal/config"
	"qrforqa/internal/middleware"
	"qrforqa/internal/services"
)

type Handler struct {
	svc *services.Service
	cfg *config.Config
}

func New(svc *services.Service, cfg *config.Config) *Handler {
	return &Handler{svc: svc, cfg: cfg}
}

// Register mounts every route on the router.
func (h *Handler) Register(r *gin.Engine) {
	r.Use(middleware.CORS(h.cfg.Dev))

	r.GET("/healthz", h.Health)

	api := r.Group("/api")

	pub := api.Group("/public")
	{
		pub.GET("/questionnaires", h.PublicList)
		pub.GET("/questionnaires/:slug", h.PublicGet)
		// Submissions and uploads are the only unauthenticated write paths, so
		// they carry the tight rate limit.
		pub.POST("/questionnaires/:slug/responses", middleware.RateLimit(1, 10), h.PublicSubmit)
		pub.POST("/questionnaires/:slug/uploads", middleware.RateLimit(1, 30), h.PublicUpload)
	}

	// Login is limited harder than anything else: it is the single credential
	// in the system.
	//
	// 0.5/s sustained with a burst of 10. The per-username lockout (8 failed
	// attempts, then a five-minute cooldown) is what actually stops password
	// guessing; this limiter is the outer layer, and an admin who mistypes a
	// few times and retries must not be locked out by their own IP.
	api.POST("/admin/login", middleware.RateLimit(0.5, 10), h.Login)

	adm := api.Group("/admin", middleware.Auth(h.svc))
	{
		adm.GET("/me", h.Me)
		adm.POST("/password", h.ChangePassword)
		adm.GET("/overview", h.Overview)
		adm.GET("/audit", h.Audit)
		adm.GET("/leaderboard", h.Leaderboard)

		adm.GET("/questionnaires", h.AdminListQuestionnaires)
		adm.POST("/questionnaires", h.AdminCreateQuestionnaire)
		adm.GET("/questionnaires/:id", h.AdminGetQuestionnaire)
		adm.PUT("/questionnaires/:id", h.AdminUpdateQuestionnaire)
		adm.DELETE("/questionnaires/:id", h.AdminDeleteQuestionnaire)
		adm.POST("/questionnaires/:id/status", h.AdminSetStatus)
		adm.GET("/questionnaires/:id/stats", h.AdminStats)
		adm.GET("/questionnaires/:id/export", h.AdminExport)
		adm.GET("/questionnaires/:id/codec", h.AdminCodecInfo)

		adm.GET("/questionnaires/:id/responses", h.AdminListResponses)
		adm.GET("/questionnaires/:id/responses/:rid", h.AdminGetResponse)
		adm.PUT("/questionnaires/:id/responses/:rid", h.AdminUpdateResponse)
		adm.DELETE("/questionnaires/:id/responses/:rid", h.AdminDeleteResponse)
		adm.GET("/questionnaires/:id/responses/:rid/raw", h.AdminRawResponse)

		adm.GET("/images/:id", h.AdminImage)

		adm.POST("/codec/encode", h.CodecEncode)
		adm.POST("/codec/decode", h.CodecDecode)
	}
}

// Health reports readiness, including that the sudoku codec still derives the
// tables it was built with. A failure here means the browser and server would
// disagree, so it is a 503 rather than a warning.
func (h *Handler) Health(c *gin.Context) {
	if err := codec.SelfTest(); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "degraded", "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status":       "ok",
		"codec":        "sudoku-v1",
		"layout":       h.cfg.Layout.String(),
		"grids":        codec.GridCount,
		"positionSets": codec.PosSetCount,
		"gridsSha256":  codec.GridsSHA256(),
	})
}

// ---------------------------------------------------------------- helpers ---

func paramUint(c *gin.Context, name string) (uint, bool) {
	v, err := strconv.ParseUint(c.Param(name), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数 " + name + " 非法"})
		return 0, false
	}
	return uint(v), true
}

func queryInt(c *gin.Context, name string, def int) int {
	v := c.Query(name)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

// fail maps a service error onto a status code, so handlers stay one-liners.
func fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "未找到"})
	case errors.Is(err, services.ErrForbidden):
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
	case errors.Is(err, services.ErrConflict):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, services.ErrInvalidCredentials):
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	}
}

// maxBodyBytes caps JSON request bodies. Ciphertext is ~4x plaintext and an
// image payload can legitimately be a couple of megabytes, so this is generous
// but bounded.
const maxBodyBytes = 12 << 20

func bindJSON(c *gin.Context, out any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBodyBytes)
	if err := c.ShouldBindJSON(out); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求内容无法解析:" + err.Error()})
		return false
	}
	return true
}
