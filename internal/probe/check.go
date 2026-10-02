// Package probe prüft, ob eigene Dienste per HTTP(S) erreichbar sind, und
// überwacht die Laufzeit ihrer TLS-Zertifikate.
package probe

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mrcdlm/dnsdeck/internal/i18n"
)

// TLSInfo beschreibt das Zertifikat, das der Server vorgelegt hat.
type TLSInfo struct {
	NotAfter time.Time
	Issuer   string
	// Err: warum das Zertifikat ungültig ist (leer = gültig).
	Err i18n.Msg
}

// Result ist das Ergebnis einer einzelnen Prüfung.
type Result struct {
	// HTTPStatus: Statuscode der Antwort (0 = keine Antwort).
	HTTPStatus int
	Latency    time.Duration
	// Err: Verbindungsfehler oder HTTP 5xx (leer = erreichbar).
	Err i18n.Msg
	TLS *TLSInfo // nil bei http:// oder ohne TLS-Verbindung
}

// Up meldet, ob der Dienst geantwortet hat (Status < 500). Ein ungültiges
// Zertifikat wird getrennt bewertet.
func (r Result) Up() bool { return r.Err.IsZero() }

// Checker führt Prüfungen aus.
type Checker struct {
	Timeout time.Duration
	// Roots: vertrauenswürdige Zertifizierungsstellen (nil = System).
	Roots *x509.CertPool
	now   func() time.Time
}

func NewChecker() *Checker { return &Checker{Timeout: 10 * time.Second, now: time.Now} }

// ValidateURL prüft eine vom Benutzer eingegebene Adresse und liefert sie
// normalisiert zurück.
func ValidateURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if len(raw) > 2048 {
		return "", i18n.E("probe.url_invalid")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil {
		return "", i18n.E("probe.url_invalid")
	}
	u.Scheme, u.Host, u.Fragment = strings.ToLower(u.Scheme), strings.ToLower(u.Host), ""
	if u.Path == "" {
		u.Path = "/"
	}
	return u.String(), nil
}

// Check ruft rawURL einmal per GET auf. Weiterleitungen werden nicht verfolgt
// (eine Weiterleitung ist eine Antwort, der Dienst also erreichbar).
func (c *Checker) Check(ctx context.Context, rawURL string) Result {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	tr := http.DefaultTransport.(*http.Transport).Clone()
	// Das Zertifikat wird unten selbst geprüft, damit Ablaufdatum und
	// Aussteller auch bei ungültigem Zertifikat sichtbar sind.
	tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	tr.DisableKeepAlives = true
	defer tr.CloseIdleConnections()
	client := &http.Client{
		Transport:     tr,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return Result{Err: i18n.M("probe.url_invalid")}
	}
	req.Header.Set("User-Agent", "dnsdeck (reachability check)")

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return Result{Latency: time.Since(start), Err: connError(err, req.URL.Hostname())}
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	res := Result{HTTPStatus: resp.StatusCode, Latency: time.Since(start)}
	if resp.StatusCode >= 500 {
		res.Err = i18n.M("probe.http", "status", strconv.Itoa(resp.StatusCode))
	}
	if resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
		res.TLS = c.verify(resp.TLS.PeerCertificates, req.URL.Hostname())
	}
	return res
}

func (c *Checker) verify(chain []*x509.Certificate, host string) *TLSInfo {
	leaf := chain[0]
	info := &TLSInfo{NotAfter: leaf.NotAfter.UTC(), Issuer: issuerName(leaf)}
	inter := x509.NewCertPool()
	for _, cert := range chain[1:] {
		inter.AddCert(cert)
	}
	now := time.Now
	if c.now != nil {
		now = c.now
	}
	_, err := leaf.Verify(x509.VerifyOptions{DNSName: host, Roots: c.Roots, Intermediates: inter, CurrentTime: now()})
	if err != nil {
		info.Err = certError(err, leaf, host, now())
	}
	return info
}

func issuerName(cert *x509.Certificate) string {
	if len(cert.Issuer.Organization) > 0 {
		if cn := cert.Issuer.CommonName; cn != "" && cn != cert.Issuer.Organization[0] {
			return cert.Issuer.Organization[0] + " (" + cn + ")"
		}
		return cert.Issuer.Organization[0]
	}
	return cert.Issuer.CommonName
}

func certError(err error, leaf *x509.Certificate, host string, now time.Time) i18n.Msg {
	var inv x509.CertificateInvalidError
	var hostErr x509.HostnameError
	var authErr x509.UnknownAuthorityError
	switch {
	case errors.As(err, &inv) && inv.Reason == x509.Expired:
		if now.Before(leaf.NotBefore) {
			return i18n.M("probe.cert_not_yet_valid")
		}
		return i18n.M("probe.cert_expired", "date", leaf.NotAfter.UTC().Format(time.DateOnly))
	case errors.As(err, &hostErr):
		return i18n.M("probe.cert_hostname", "host", host)
	case errors.As(err, &authErr):
		return i18n.M("probe.cert_untrusted")
	}
	return i18n.M("probe.cert_invalid", "detail", err.Error())
}

// connError übersetzt typische Verbindungsfehler in verständliche Meldungen.
func connError(err error, host string) i18n.Msg {
	var dnsErr *net.DNSError
	var certErr *tls.CertificateVerificationError
	switch {
	case errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded):
		return i18n.M("probe.timeout")
	case errors.As(err, &dnsErr):
		if dnsErr.IsNotFound {
			return i18n.M("probe.dns", "host", host)
		}
		return i18n.M("probe.failed", "detail", dnsErr.Error())
	case errors.Is(err, syscall.ECONNREFUSED):
		return i18n.M("probe.refused")
	case errors.As(err, &certErr):
		return i18n.M("probe.tls_failed", "detail", certErr.Err.Error())
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return i18n.M("probe.timeout")
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		err = urlErr.Err
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) && opErr.Op == "remote error" || strings.Contains(err.Error(), "tls:") {
		return i18n.M("probe.tls_failed", "detail", err.Error())
	}
	return i18n.M("probe.failed", "detail", err.Error())
}
