package ipdetect

import (
	"net/netip"
	"strconv"

	"github.com/mrcdlm/dnsdeck/internal/i18n"
)

// Decision ist das Ergebnis des Mehrheitsentscheids.
type Decision struct {
	// IP ist die bestätigte Adresse; ungültig, wenn keine Entscheidung möglich war.
	IP     netip.Addr
	Votes  int      // Stimmen für IP
	Total  int      // Anzahl gültiger Antworten
	Reason i18n.Msg // Begründung, falls nicht bestätigt
}

func (d Decision) Confirmed() bool { return d.IP.IsValid() }

// Decide wählt aus den gültigen Antworten die öffentliche IP:
//   - ≥ 2 Antworten: der häufigste Wert braucht eine echte Mehrheit und
//     mindestens 2 Stimmen.
//   - genau 1 Antwort: wird nur akzeptiert, wenn noch keine IP bekannt ist
//     oder sie mit der bekannten IP übereinstimmt (kein Wechsel auf Basis
//     einer einzelnen Quelle).
//   - 0 Antworten: keine Entscheidung.
func Decide(addrs []netip.Addr, known netip.Addr) Decision {
	n := len(addrs)
	if n == 0 {
		return Decision{Reason: i18n.M("ip.no_source")}
	}

	counts := map[netip.Addr]int{}
	var best netip.Addr
	for _, a := range addrs {
		counts[a]++
		if counts[a] > counts[best] {
			best = a
		}
	}
	votes := counts[best]

	if n == 1 {
		if !known.IsValid() || known == best {
			return Decision{IP: best, Votes: 1, Total: 1}
		}
		return Decision{Votes: 1, Total: 1,
			Reason: i18n.M("ip.single_source", "ip", best.String())}
	}
	if votes >= 2 && votes*2 > n {
		return Decision{IP: best, Votes: votes, Total: n}
	}
	return Decision{Votes: votes, Total: n,
		Reason: i18n.M("ip.no_majority", "votes", strconv.Itoa(votes), "total", strconv.Itoa(n), "ip", best.String())}
}
