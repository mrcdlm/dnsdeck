package probe

import (
	"time"

	"github.com/mrcdlm/dnsdeck/internal/store"
	"github.com/mrcdlm/dnsdeck/internal/tunnels"
)

// Zeiträume der Uptime-Balken – wie bei den Tunnels.
var ranges = []struct {
	key     string
	d       time.Duration
	buckets int
}{
	{"24h", 24 * time.Hour, 48},    // 30 min je Stück
	{"7d", 7 * 24 * time.Hour, 56}, // 3 h je Stück
}

// LongestRange: so weit reicht der Verlauf zurück, der für Uptimes gebraucht wird.
var LongestRange = ranges[len(ranges)-1].d

// barStatus bildet den Status einer Prüfung auf die Begriffe der Uptime-Balken
// ab (dieselben wie bei Tunnels): ablaufendes Zertifikat = eingeschränkt, aber
// erreichbar; ungültiges Zertifikat = Störung.
func barStatus(s string) string {
	switch s {
	case store.ProbeUp:
		return tunnels.StatusHealthy
	case store.ProbeExpiring:
		return tunnels.StatusDegraded
	case store.ProbeDown, store.ProbeTLSError:
		return tunnels.StatusDown
	}
	return tunnels.StatusInactive
}

// Uptimes berechnet die Balken für 24 h und 7 Tage. interval ist der Abstand
// der Prüfungen (so lange gilt eine Beobachtung).
func Uptimes(segs []store.Segment, now time.Time, interval time.Duration) map[string]tunnels.Uptime {
	mapped := make([]store.Segment, len(segs))
	for i, s := range segs {
		s.Status = barStatus(s.Status)
		mapped[i] = s
	}
	out := make(map[string]tunnels.Uptime, len(ranges))
	for _, r := range ranges {
		out[r.key] = tunnels.ComputeUptime(mapped, now.Add(-r.d), now, r.buckets, interval)
	}
	return out
}
