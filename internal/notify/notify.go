// Package notify verschickt Benachrichtigungen über frei konfigurierbare
// Webhooks (Methode, URL, Header, Body-Template).
//
// Geheimnisse stehen nie in der Konfiguration selbst, sondern als Platzhalter:
// ${WEBHOOK_NAME} in URL und Headern, {{env "WEBHOOK_NAME"}} im Body-Template.
// Die Werte kommen aus Env-Variablen; erlaubt sind nur Variablen mit dem
// Präfix WEBHOOK_, damit über die Oberfläche z. B. CF_API_TOKEN nicht an
// einen fremden Server geschickt werden kann.
package notify

import "time"

// Ereignistypen, die einzeln an- und abgeschaltet werden können.
const (
	EventIPChange        = "ip_change"
	EventUpdateFailed    = "update_failed"
	EventUpdateRecovered = "update_recovered"
	EventTunnelStatus    = "tunnel_status"
	EventTest            = "test" // Testnachricht, immer zugestellt
)

// EventTypes in Anzeigereihenfolge.
var EventTypes = []string{EventIPChange, EventUpdateFailed, EventUpdateRecovered, EventTunnelStatus}

// Priorität (angelehnt an ntfy: 1 min … 5 max).
const (
	PriorityLow     = 2
	PriorityDefault = 3
	PriorityHigh    = 4
)

type Event struct {
	Type     string            `json:"type"`
	Title    string            `json:"title"`
	Message  string            `json:"message"`
	Priority int               `json:"priority"`
	Time     time.Time         `json:"time"`
	Data     map[string]string `json:"data,omitempty"`
}

// Notifier nimmt Ereignisse entgegen (nil-sicher über Send).
type Notifier interface {
	Notify(ev Event)
}

// Send leitet ev an n weiter, wenn n gesetzt ist.
func Send(n Notifier, ev Event) {
	if n != nil {
		n.Notify(ev)
	}
}
