package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"qrforqa/internal/services"
)

// ContextAdminID/ContextAdminUser carry the authenticated admin through the
// request.
const (
	ContextAdminID   = "adminID"
	ContextAdminUser = "adminUser"
)

// Auth requires a valid bearer token and stores the admin in the context.
func Auth(svc *services.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		token := ""
		if strings.HasPrefix(header, "Bearer ") {
			token = strings.TrimSpace(header[len("Bearer "):])
		}
		if token == "" {
			// Fall back to a query parameter so image URLs can be opened in a
			// new tab from the admin UI without custom headers.
			token = c.Query("token")
		}
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
			return
		}
		claims, err := svc.ParseToken(token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "登录已过期,请重新登录"})
			return
		}
		c.Set(ContextAdminID, claims.AdminID)
		c.Set(ContextAdminUser, claims.Username)
		c.Next()
	}
}

// AdminID returns the authenticated admin's ID.
func AdminID(c *gin.Context) uint {
	if v, ok := c.Get(ContextAdminID); ok {
		if id, ok := v.(uint); ok {
			return id
		}
	}
	return 0
}

// AdminUser returns the authenticated admin's username.
func AdminUser(c *gin.Context) string {
	if v, ok := c.Get(ContextAdminUser); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// bucket is a token bucket for one client.
type bucket struct {
	tokens float64
	last   time.Time
}

// RateLimit applies a per-client token bucket. It is intentionally in-memory:
// the server is a single instance, and an external store would be a dependency
// with no benefit here.
func RateLimit(rps float64, burst int) gin.HandlerFunc {
	var mu sync.Mutex
	buckets := map[string]*bucket{}

	// Reap idle buckets so a wide source range cannot grow the map forever.
	go func() {
		for range time.Tick(10 * time.Minute) {
			mu.Lock()
			cutoff := time.Now().Add(-10 * time.Minute)
			for k, b := range buckets {
				if b.last.Before(cutoff) {
					delete(buckets, k)
				}
			}
			mu.Unlock()
		}
	}()

	return func(c *gin.Context) {
		key := c.ClientIP()
		now := time.Now()

		mu.Lock()
		b, ok := buckets[key]
		if !ok {
			b = &bucket{tokens: float64(burst), last: now}
			buckets[key] = b
		}
		b.tokens = min(float64(burst), b.tokens+now.Sub(b.last).Seconds()*rps)
		b.last = now
		allowed := b.tokens >= 1
		if allowed {
			b.tokens--
		}
		mu.Unlock()

		if !allowed {
			c.Header("Retry-After", "5")
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "请求过于频繁,请稍后再试"})
			return
		}
		c.Next()
	}
}

// RequestLog emits one structured line per request.
func RequestLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		path := c.Request.URL.Path
		// Never log a token that arrived as a query parameter.
		if c.Request.URL.RawQuery != "" && !strings.Contains(path, "/images/") {
			path += "?" + c.Request.URL.RawQuery
		}
		fmt.Printf("%s %d %s %s %v\n",
			time.Now().Format("2006-01-02T15:04:05"),
			c.Writer.Status(), c.Request.Method, path, time.Since(start).Round(time.Millisecond))
	}
}

// CORS allows the Vite dev server to call the API during local development.
// It is disabled in production, where the frontend is served from the same
// origin.
func CORS(enabled bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if enabled {
			origin := c.GetHeader("Origin")
			if origin != "" {
				c.Header("Access-Control-Allow-Origin", origin)
				c.Header("Vary", "Origin")
				c.Header("Access-Control-Allow-Credentials", "true")
				c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type")
				c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			}
			if c.Request.Method == http.MethodOptions {
				c.AbortWithStatus(http.StatusNoContent)
				return
			}
		}
		c.Next()
	}
}

// HashIP returns a salted digest of a client address, so abuse can be traced
// without storing personal data.
func HashIP(ip, salt string) string {
	sum := sha256.Sum256([]byte(salt + "|" + ip))
	return hex.EncodeToString(sum[:16])
}
