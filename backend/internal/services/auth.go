package services

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"qrforqa/internal/models"
)

// ErrInvalidCredentials is returned for both an unknown username and a wrong
// password, so the endpoint does not confirm which usernames exist.
var ErrInvalidCredentials = errors.New("用户名或密码不正确")

// EnsureAdmin seeds the administrator account on first run.
//
// It never overwrites an existing account: re-running with a different
// QRFORQA_ADMIN_PASSWORD must not silently reset the password.
func (s *Service) EnsureAdmin() (created bool, err error) {
	var count int64
	if err := s.db.Model(&models.Admin{}).Count(&count).Error; err != nil {
		return false, err
	}
	if count > 0 {
		return false, nil
	}
	if err := s.cfg.FirstRunCredentialsOK(); err != nil {
		return false, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(s.cfg.AdminPassword), bcrypt.DefaultCost)
	if err != nil {
		return false, err
	}
	admin := &models.Admin{Username: s.cfg.AdminUser, PasswordHash: string(hash)}
	if err := s.db.Create(admin).Error; err != nil {
		return false, err
	}
	return true, nil
}

// Authenticate verifies credentials, applying a per-username lockout so the
// single admin account cannot be brute-forced.
func (s *Service) Authenticate(username, password string) (*models.Admin, error) {
	if !s.loginAllowed(username) {
		return nil, fmt.Errorf("尝试次数过多,请稍后再试")
	}

	var admin models.Admin
	err := s.db.Where("username = ?", username).First(&admin).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Spend comparable time on a missing user so the response timing
			// does not reveal whether the username exists.
			_ = bcrypt.CompareHashAndPassword(
				[]byte("$2a$10$CwTycUXWue0Thq9StjUM0uJ8i9jVJ0z.HXQKz8jVQ5Y5Y5Y5Y5Y5u"), []byte(password))
			s.recordLoginFailure(username)
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(password)); err != nil {
		s.recordLoginFailure(username)
		return nil, ErrInvalidCredentials
	}
	s.clearLoginFailures(username)
	return &admin, nil
}

// ChangePassword updates the admin's password after verifying the current one.
func (s *Service) ChangePassword(id uint, current, next string) error {
	if len(next) < 8 {
		return errors.New("新密码至少需要 8 位")
	}
	var admin models.Admin
	if err := s.db.First(&admin, id).Error; err != nil {
		return err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(current)); err != nil {
		return ErrInvalidCredentials
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(next), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.db.Model(&admin).Update("password_hash", string(hash)).Error
}

// ------------------------------------------------------------------ JWT ----

// Claims is the admin session token payload.
type Claims struct {
	AdminID  uint   `json:"aid"`
	Username string `json:"usr"`
	jwt.RegisteredClaims
}

// IssueToken mints a session token.
func (s *Service) IssueToken(admin *models.Admin) (string, time.Time, error) {
	expires := time.Now().Add(s.cfg.TokenTTL)
	claims := Claims{
		AdminID:  admin.ID,
		Username: admin.Username,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   admin.Username,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(expires),
			Issuer:    "qrforqa",
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString([]byte(s.cfg.JWTSecret))
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, expires, nil
}

// ParseToken validates a session token.
func (s *Service) ParseToken(raw string) (*Claims, error) {
	claims := &Claims{}
	tok, err := jwt.ParseWithClaims(raw, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method %v", t.Header["alg"])
		}
		return []byte(s.cfg.JWTSecret), nil
	}, jwt.WithIssuer("qrforqa"), jwt.WithValidMethods([]string{"HS256"}))
	if err != nil {
		return nil, err
	}
	if !tok.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}

// ------------------------------------------------------- login throttle ----

type loginState struct {
	failures int
	until    time.Time
}

var loginThrottle = struct {
	mu    sync.Mutex
	state map[string]*loginState
}{state: map[string]*loginState{}}

const (
	maxLoginFailures = 8
	loginLockout     = 5 * time.Minute
)

func (s *Service) loginAllowed(username string) bool {
	loginThrottle.mu.Lock()
	defer loginThrottle.mu.Unlock()
	st, ok := loginThrottle.state[username]
	if !ok {
		return true
	}
	if time.Now().After(st.until) {
		delete(loginThrottle.state, username)
		return true
	}
	return st.failures < maxLoginFailures
}

func (s *Service) recordLoginFailure(username string) {
	loginThrottle.mu.Lock()
	defer loginThrottle.mu.Unlock()
	st, ok := loginThrottle.state[username]
	if !ok {
		st = &loginState{}
		loginThrottle.state[username] = st
	}
	st.failures++
	if st.failures >= maxLoginFailures {
		st.until = time.Now().Add(loginLockout)
	}
}

func (s *Service) clearLoginFailures(username string) {
	loginThrottle.mu.Lock()
	defer loginThrottle.mu.Unlock()
	delete(loginThrottle.state, username)
}
