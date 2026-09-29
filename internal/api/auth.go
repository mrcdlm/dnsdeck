package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	sessionCookie = "dnsdeck_session"
	sessionTTL    = 30 * 24 * time.Hour

	// Fehlversuche pro Client-Adresse und Zeitfenster, danach 429.
	maxFailures   = 10
	failureWindow = time.Minute
	failureDelay  = time.Second
)

type sessionStore interface {
	CreateSession(ctx context.Context, tokenHash string, expiresAt time.Time) error
	SessionValid(ctx context.Context, tokenHash string, now time.Time) (bool, error)
	DeleteSession(ctx context.Context, tokenHash string) error
}

// Auth prüft das Passwort und verwaltet Sessions.
type Auth struct {
	hash  []byte
	store sessionStore
	now   func() time.Time
	delay time.Duration

	mu       sync.Mutex
	failures map[string][]time.Time
}

// NewAuth erwartet APP_PASSWORD. Ist der Wert bereits ein bcrypt-Hash, wird
// er direkt verwendet; sonst wird er beim Start gehasht, damit der Vergleich
// immer über bcrypt läuft und das Klartext-Passwort nicht im Speicher der
// Auth-Komponente verbleibt.
func NewAuth(password string, st sessionStore) (*Auth, error) {
	a := &Auth{store: st, now: time.Now, delay: failureDelay, failures: map[string][]time.Time{}}
	if strings.HasPrefix(password, "$2") {
		if _, err := bcrypt.Cost([]byte(password)); err == nil {
			a.hash = []byte(password)
			return a, nil
		}
	}
	if len(password) > 72 {
		return nil, errors.New("APP_PASSWORD darf höchstens 72 Bytes lang sein (bcrypt-Grenze)")
	}
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	a.hash = h
	return a, nil
}

func (a *Auth) checkPassword(pw string) bool {
	return bcrypt.CompareHashAndPassword(a.hash, []byte(pw)) == nil
}

// tooManyFailures meldet, ob der Client gesperrt ist, und räumt alte Einträge auf.
func (a *Auth) tooManyFailures(client string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	cutoff := a.now().Add(-failureWindow)
	for k, ts := range a.failures {
		kept := ts[:0]
		for _, t := range ts {
			if t.After(cutoff) {
				kept = append(kept, t)
			}
		}
		if len(kept) == 0 {
			delete(a.failures, k)
		} else {
			a.failures[k] = kept
		}
	}
	return len(a.failures[client]) >= maxFailures
}

func (a *Auth) recordFailure(client string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.failures[client] = append(a.failures[client], a.now())
}

func (a *Auth) newSession(ctx context.Context) (token string, expires time.Time, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", time.Time{}, err
	}
	token = base64.RawURLEncoding.EncodeToString(b)
	expires = a.now().Add(sessionTTL)
	return token, expires, a.store.CreateSession(ctx, hashToken(token), expires)
}

func (a *Auth) valid(r *http.Request) (bool, error) {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return false, nil
	}
	return a.store.SessionValid(r.Context(), hashToken(c.Value), a.now())
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func sessionCookieFor(r *http.Request, value string, expires time.Time) *http.Cookie {
	c := &http.Cookie{
		Name:     sessionCookie,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   isHTTPS(r),
	}
	if value == "" {
		c.MaxAge = -1
	} else {
		c.Expires = expires
	}
	return c
}

// isHTTPS: direkt per TLS oder hinter einem Proxy (z. B. Cloudflare Tunnel).
func isHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func clientKey(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
