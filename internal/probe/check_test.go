package probe

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// newCert erzeugt ein selbst signiertes Serverzertifikat.
func newCert(t *testing.T, notBefore, notAfter time.Time, dnsNames []string, ips ...net.IP) (tls.Certificate, *x509.Certificate) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: "dnsdeck test"},
		Issuer:       pkix.Name{CommonName: "dnsdeck test"},
		NotBefore:    notBefore, NotAfter: notAfter,
		DNSNames: dnsNames, IPAddresses: ips,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true, IsCA: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, cert
}

func tlsServer(t *testing.T, cert tls.Certificate, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewUnstartedServer(h)
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{cert}}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv
}

func ok(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }

func checkerTrusting(certs ...*x509.Certificate) *Checker {
	pool := x509.NewCertPool()
	for _, c := range certs {
		pool.AddCert(c)
	}
	return &Checker{Timeout: 2 * time.Second, Roots: pool}
}

var localhostIP = net.ParseIP("127.0.0.1")

func TestCheckValidCertificate(t *testing.T) {
	notAfter := time.Now().Add(90 * 24 * time.Hour).Truncate(time.Second)
	cert, leaf := newCert(t, time.Now().Add(-time.Hour), notAfter, nil, localhostIP)
	srv := tlsServer(t, cert, ok)

	res := checkerTrusting(leaf).Check(t.Context(), srv.URL+"/")
	if !res.Up() || res.HTTPStatus != 200 || res.TLS == nil || !res.TLS.Err.IsZero() {
		t.Fatalf("%+v %+v", res, res.TLS)
	}
	if !res.TLS.NotAfter.Equal(notAfter) || res.TLS.Issuer != "dnsdeck test" {
		t.Fatalf("Zertifikat: %+v", res.TLS)
	}
}

func TestCheckInvalidCertificates(t *testing.T) {
	now := time.Now()
	expired, expiredLeaf := newCert(t, now.Add(-48*time.Hour), now.Add(-24*time.Hour), nil, localhostIP)
	wrongHost, wrongHostLeaf := newCert(t, now.Add(-time.Hour), now.Add(24*time.Hour), []string{"other.example"})
	untrusted, _ := newCert(t, now.Add(-time.Hour), now.Add(24*time.Hour), nil, localhostIP)

	cases := []struct {
		name  string
		cert  tls.Certificate
		trust []*x509.Certificate
		code  string
	}{
		{"abgelaufen", expired, []*x509.Certificate{expiredLeaf}, "probe.cert_expired"},
		{"falscher Host", wrongHost, []*x509.Certificate{wrongHostLeaf}, "probe.cert_hostname"},
		{"unbekannte Stelle", untrusted, nil, "probe.cert_untrusted"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := tlsServer(t, c.cert, ok)
			res := checkerTrusting(c.trust...).Check(t.Context(), srv.URL)
			// Der Dienst antwortet – nur das Zertifikat ist ungültig.
			if !res.Up() || res.TLS == nil || res.TLS.Err.Code != c.code {
				t.Fatalf("%+v %+v", res, res.TLS)
			}
		})
	}
}

func TestCheckHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/kaputt":
			w.WriteHeader(http.StatusServiceUnavailable)
		case "/weiter":
			http.Redirect(w, r, "/kaputt", http.StatusFound)
		case "/geheim":
			w.WriteHeader(http.StatusUnauthorized)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()
	c := &Checker{Timeout: 2 * time.Second}

	if res := c.Check(t.Context(), srv.URL+"/"); !res.Up() || res.HTTPStatus != 200 || res.TLS != nil {
		t.Errorf("ok: %+v", res)
	}
	if res := c.Check(t.Context(), srv.URL+"/kaputt"); res.Up() || res.HTTPStatus != 503 || res.Err.Code != "probe.http" {
		t.Errorf("5xx: %+v", res)
	}
	// Weiterleitung wird nicht verfolgt – sonst wäre das Ergebnis 503.
	if res := c.Check(t.Context(), srv.URL+"/weiter"); !res.Up() || res.HTTPStatus != 302 {
		t.Errorf("Weiterleitung: %+v", res)
	}
	// Login-Seiten u. ä. gelten als erreichbar.
	if res := c.Check(t.Context(), srv.URL+"/geheim"); !res.Up() || res.HTTPStatus != 401 {
		t.Errorf("401: %+v", res)
	}
}

func TestCheckConnectionErrors(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closed := "http://" + ln.Addr().String() + "/"
	ln.Close()
	c := &Checker{Timeout: 2 * time.Second}
	if res := c.Check(t.Context(), closed); res.Up() || res.Err.Code != "probe.refused" {
		t.Errorf("abgelehnt: %+v", res)
	}

	block := make(chan struct{})
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-block:
		case <-r.Context().Done():
		}
	}))
	defer slow.Close()
	defer close(block)
	c.Timeout = 100 * time.Millisecond
	if res := c.Check(t.Context(), slow.URL); res.Up() || res.Err.Code != "probe.timeout" {
		t.Errorf("Zeitüberschreitung: %+v", res)
	}

	c.Timeout = 2 * time.Second
	if res := c.Check(t.Context(), "https://gibt-es-nicht.invalid/"); res.Up() || res.Err.Code != "probe.dns" {
		t.Errorf("DNS: %+v", res)
	}
}

func TestValidateURL(t *testing.T) {
	good := map[string]string{
		"https://NAS.Example.com":        "https://nas.example.com/",
		"nas.example.com":                "https://nas.example.com/",
		" http://192.168.1.2:8080/x#a ":  "http://192.168.1.2:8080/x",
		"https://[2001:db8::1]/status?a": "https://[2001:db8::1]/status?a",
	}
	for in, want := range good {
		if got, err := ValidateURL(in); err != nil || got != want {
			t.Errorf("%q → %q, %v (erwartet %q)", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "ftp://x.example", "https://", "https://user:pw@x.example/", "https://x y"} {
		if _, err := ValidateURL(bad); err == nil {
			t.Errorf("%q: Fehler erwartet", bad)
		}
	}
}
