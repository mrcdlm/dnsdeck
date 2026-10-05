package api

import (
	"context"
	"crypto/hmac"
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
	hash []byte
	// sessionKey bindet Sessions an APP_PASSWORD: gespeichert wird nur
	// HMAC(sessionKey, Token). Ändert sich das Passwort, passt kein
	// gespeicherter Wert mehr – alle Sessions sind sofort ungültig.
	sessionKey []byte
	store      sessionStore
	now        func() time.Time
	delay      time.Duration

	mu       sync.Mutex
	attempts map[string][]time.Time // Anmeldeversuche je Client im Zeitfenster
}

// NewAuth erwartet APP_PASSWORD. Ist der Wert bereits ein bcrypt-Hash, wird
// er direkt verwendet; sonst wird er beim Start gehasht, damit der Vergleich
// immer über bcrypt läuft und das Klartext-Passwort nicht im Speicher der
// Auth-Komponente verbleibt.
func NewAuth(password string, st sessionStore) (*Auth, error) {
	key := sha256.Sum256([]byte("dnsdeck session key\x00" + password))
	a := &Auth{store: st, now: time.Now, delay: failureDelay, attempts: map[string][]time.Time{},
		sessionKey: key[:]}
	if strings.HasPrefix(password, "$2") {
		if _, err := bcrypt.Cost([]byte(password)); err == nil {
			a.hash = []byte(password)
			return a, nil
		}
	}
	if len(password) > 72 {
		return nil, errors.New("APP_PASSWORD must not be longer than 72 bytes (bcrypt limit)")
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

// beginAttempt zählt einen Anmeldeversuch, bevor das Passwort geprüft wird,
// und meldet false, wenn der Client sein Kontingent ausgeschöpft hat. Weil
// schon der Versuch zählt, hebeln parallele Anfragen die Grenze nicht aus.
func (a *Auth) beginAttempt(client string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	cutoff := a.now().Add(-failureWindow)
	for k, ts := range a.attempts {
		kept := ts[:0]
		for _, t := range ts {
			if t.After(cutoff) {
				kept = append(kept, t)
			}
		}
		if len(kept) == 0 {
			delete(a.attempts, k)
		} else {
			a.attempts[k] = kept
		}
	}
	if len(a.attempts[client]) >= maxFailures {
		return false
	}
	a.attempts[client] = append(a.attempts[client], a.now())
	return true
}

// loginSucceeded vergisst die Versuche des Clients.
func (a *Auth) loginSucceeded(client string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.attempts, client)
}

func (a *Auth) newSession(ctx context.Context) (token string, expires time.Time, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", time.Time{}, err
	}
	token = base64.RawURLEncoding.EncodeToString(b)
	expires = a.now().Add(sessionTTL)
	return token, expires, a.store.CreateSession(ctx, a.hashToken(token), expires)
}

func (a *Auth) valid(r *http.Request) (bool, error) {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return false, nil
	}
	return a.store.SessionValid(r.Context(), a.hashToken(c.Value), a.now())
}

// hashToken liefert den gespeicherten Wert eines Tokens. Das Token selbst steht
// nie in der DB; ohne Token lässt sich aus dem HMAC nichts über das Passwort
// ableiten.
func (a *Auth) hashToken(token string) string {
	m := hmac.New(sha256.New, a.sessionKey)
	m.Write([]byte(token))
	return hex.EncodeToString(m.Sum(nil))
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
