package tunnels

import (
	"time"

	"github.com/mrcdlm/dnsdeck/internal/store"
)

// Tunnel-Status laut Cloudflare.
const (
	StatusHealthy  = "healthy"
	StatusDegraded = "degraded"
	StatusDown     = "down"
	StatusInactive = "inactive"
)

// severity bestimmt, welcher Status einen Balkenabschnitt einfärbt, wenn darin
// mehrere vorkommen: der schlechteste gewinnt.
var severity = map[string]int{StatusHealthy: 1, StatusInactive: 2, StatusDegraded: 3, StatusDown: 4}

// isUp: degraded (nicht alle Verbindungen aktiv) ist noch erreichbar.
func isUp(status string) bool { return status == StatusHealthy || status == StatusDegraded }

// Uptime beschreibt einen Zeitraum als Balken aus gleich langen Abschnitten.
type Uptime struct {
	// Percent: Anteil erreichbarer Zeit an der beobachteten Zeit; nil, wenn
	// im Zeitraum nichts beobachtet wurde.
	Percent *float64 `json:"percent"`
	// Observed: Anteil beobachteter Zeit am Zeitraum (0..1).
	Observed float64   `json:"observed"`
	Start    time.Time `json:"start"`
	// BucketSeconds: Länge eines Abschnitts.
	BucketSeconds int `json:"bucket_seconds"`
	// Buckets: schlechtester Status je Abschnitt, "" = unbekannt.
	Buckets []string `json:"buckets"`
}

// ComputeUptime verteilt die Abschnitte auf n Balkenstücke im Zeitraum
// [from, to]. Eine Beobachtung gilt bis zur nächsten Abfrage (valid), aber
// höchstens bis zum Beginn des nächsten Abschnitts – Lücken bleiben unbekannt.
func ComputeUptime(segs []store.TunnelSegment, from, to time.Time, n int, valid time.Duration) Uptime {
	bucket := to.Sub(from) / time.Duration(n)
	u := Uptime{Start: from, BucketSeconds: int(bucket.Seconds()), Buckets: make([]string, n)}

	var up, known time.Duration
	for i, seg := range segs {
		s := seg.StartedAt
		e := seg.LastSeenAt.Add(valid)
		if i+1 < len(segs) && segs[i+1].StartedAt.Before(e) {
			e = segs[i+1].StartedAt
		}
		s, e = maxTime(s, from), minTime(e, to)
		if !e.After(s) {
			continue
		}
		known += e.Sub(s)
		if isUp(seg.Status) {
			up += e.Sub(s)
		}
		// betroffene Balkenstücke
		first := int(s.Sub(from) / bucket)
		last := int((e.Sub(from) - 1) / bucket)
		for b := max(first, 0); b <= min(last, n-1); b++ {
			if severity[seg.Status] > severity[u.Buckets[b]] {
				u.Buckets[b] = seg.Status
			}
		}
	}
	if known > 0 {
		p := float64(up) / float64(known) * 100
		u.Percent = &p
	}
	u.Observed = float64(known) / float64(to.Sub(from))
	return u
}

// StatusSince liefert den Beginn der letzten zusammenhängenden Folge von
// Abschnitten mit status (Lücken bis maxGap gelten als zusammenhängend).
func StatusSince(segs []store.TunnelSegment, status string, maxGap time.Duration) *time.Time {
	var since *time.Time
	for i := len(segs) - 1; i >= 0; i-- {
		seg := segs[i]
		if seg.Status != status {
			break
		}
		if since != nil && since.Sub(seg.LastSeenAt) > maxGap {
			break
		}
		t := seg.StartedAt
		since = &t
	}
	return since
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
