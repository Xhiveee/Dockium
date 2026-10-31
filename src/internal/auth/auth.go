package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"

	"golang.org/x/crypto/bcrypt"
)

const (
	SessionCookieName = "dockium_session"
	SessionTTL        = 7 * 24 * time.Hour
	MinPasswordLength = 10
	MaxPasswordLength = 72
)

var (
	ErrPasswordTooShort = errors.New("пароль должен содержать не менее 10 символов")
	ErrPasswordTooLong  = errors.New("пароль слишком длинный")
	ErrPasswordWeak     = errors.New("пароль должен содержать буквы и цифры")
	ErrLoginInvalid     = errors.New("логин: 3–32 символа, латиница, цифры, точка, дефис или подчёркивание")
)

var loginPattern = regexp.MustCompile(`^[a-zA-Z0-9_.-]{3,32}$`)

func ValidateLogin(login string) error {
	if !loginPattern.MatchString(login) {
		return ErrLoginInvalid
	}
	return nil
}

func ValidatePassword(password, login string) error {
	if len(password) < MinPasswordLength {
		return ErrPasswordTooShort
	}
	if len(password) > MaxPasswordLength {
		return ErrPasswordTooLong
	}
	hasLetter, hasDigit := false, false
	for _, symbol := range password {
		if unicode.IsLetter(symbol) {
			hasLetter = true
		}
		if unicode.IsDigit(symbol) {
			hasDigit = true
		}
	}
	if !hasLetter || !hasDigit {
		return ErrPasswordWeak
	}
	if len(login) > 0 && strings.EqualFold(password, login) {
		return ErrPasswordWeak
	}
	return nil
}

func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func VerifyPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

var (
	dummyOnce sync.Once
	dummyHash string
)

func VerifyDummyPassword(password string) {
	dummyOnce.Do(func() {
		dummyHash, _ = HashPassword("dockium-placeholder-password")
	})
	if dummyHash != "" {
		_ = bcrypt.CompareHashAndPassword([]byte(dummyHash), []byte(password))
	}
}

func NewToken() (raw string, hash string, err error) {
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		return "", "", err
	}
	raw = "dk_" + base64.RawURLEncoding.EncodeToString(buffer)
	return raw, TokenHash(raw), nil
}

func TokenHash(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func TokenEqual(hashA, hashB string) bool {
	return subtle.ConstantTimeCompare([]byte(hashA), []byte(hashB)) == 1
}

func SessionID() (string, error) {
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func HashSessionID(id string) string {
	sum := sha256.Sum256([]byte(id))
	return hex.EncodeToString(sum[:])
}

func ParseUserAgent(value string) (browser, os string) {
	agent := strings.ToLower(value)
	switch {
	case strings.Contains(agent, "edg/"):
		browser = "Edge"
	case strings.Contains(agent, "opr/") || strings.Contains(agent, "opera"):
		browser = "Opera"
	case strings.Contains(agent, "firefox/"):
		browser = "Firefox"
	case strings.Contains(agent, "chrome/") || strings.Contains(agent, "chromium"):
		browser = "Chrome"
	case strings.Contains(agent, "safari/"):
		browser = "Safari"
	case strings.Contains(agent, "curl/"):
		browser = "curl"
	case strings.Contains(agent, "wget/"):
		browser = "wget"
	default:
		browser = "Браузер"
	}
	switch {
	case strings.Contains(agent, "windows"):
		os = "Windows"
	case strings.Contains(agent, "android"):
		os = "Android"
	case strings.Contains(agent, "iphone") || strings.Contains(agent, "ipad"):
		os = "iOS"
	case strings.Contains(agent, "mac os") || strings.Contains(agent, "macintosh"):
		os = "macOS"
	case strings.Contains(agent, "linux"):
		os = "Linux"
	default:
		os = "Неизвестно"
	}
	return browser, os
}

func TimeNow() int64 {
	return time.Now().UTC().UnixMilli()
}
