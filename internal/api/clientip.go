package api

import (
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strings"
)

// clientIP liefert die Adresse des Clients – für Login-Sperre und Logs.
//
// Kommt die Anfrage von einem vertrauenswürdigen Proxy (TRUSTED_PROXIES), gilt
// CF-Connecting-IP (Cloudflare Tunnel) bzw. der letzte nicht vertrauenswürdige
// Eintrag in X-Forwarded-For. Allen anderen wird nicht geglaubt: Diese Header
// kann jeder Client selbst setzen, um sich als jemand anderes auszugeben.
func (s *Server) clientIP(r *http.Request) string {
	remote := remoteAddr(r)
	if !s.trustedProxy(remote) {
		return addrString(remote, r.RemoteAddr)
	}
	if a, err := netip.ParseAddr(strings.TrimSpace(r.Header.Get("CF-Connecting-IP"))); err == nil {
		return a.Unmap().String()
	}
	// X-Forwarded-For: "client, proxy1, proxy2" – von rechts lesen und vertrauenswürdige
	// Proxys überspringen; links davon kann der Client beliebiges eingetragen haben.
	var hops []string
	for _, h := range r.Header.Values("X-Forwarded-For") {
		hops = append(hops, strings.Split(h, ",")...)
	}
	for _, h := range slices.Backward(hops) {
		a, err := netip.ParseAddr(strings.TrimSpace(h))
		if err != nil {
			break
		}
		if a = a.Unmap(); !s.trustedProxy(a) {
			return a.String()
		}
	}
	return addrString(remote, r.RemoteAddr)
}

func (s *Server) trustedProxy(a netip.Addr) bool {
	if !a.IsValid() {
		return false
	}
	for _, p := range s.trustedProxies {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

func remoteAddr(r *http.Request) netip.Addr {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	a, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}
	}
	return a.Unmap()
}

func addrString(a netip.Addr, fallback string) string {
	if a.IsValid() {
		return a.String()
	}
	return fallback
}
