// Package events verteilt Änderungshinweise an alle verbundenen Clients
// (Server-Sent Events). Ereignisse tragen nur das Thema – der Client lädt die
// betroffenen Daten daraufhin über die normale API neu. So gelangen keine
// Daten an der Authentifizierung/Filterung der API vorbei.
package events

import "sync"

// Themen, auf die das Frontend reagiert.
const (
	TopicIP        = "ip"
	TopicRecords   = "records"
	TopicUpdates   = "updates"
	TopicTunnels   = "tunnels"
	TopicSettings  = "settings"
	TopicWebhooks  = "webhooks"
	TopicProbes    = "probes"
	TopicSpeedtest = "speedtest"
)

// Publisher ist das, was Produzenten von Ereignissen brauchen. Ein nil-Publisher
// ist erlaubt (siehe Publish).
type Publisher interface {
	Publish(topic string)
}

// Publish ruft p.Publish auf, wenn p gesetzt ist.
func Publish(p Publisher, topic string) {
	if p != nil {
		p.Publish(topic)
	}
}

// Broker verteilt Themen an Abonnenten. Langsame Abonnenten blockieren nicht:
// ist ihr Puffer voll, wird das Ereignis für sie verworfen (das nächste
// Ereignis desselben Themas oder das Nachladen nach Reconnect holen es nach).
type Broker struct {
	mu     sync.Mutex
	subs   map[chan string]struct{}
	closed bool
}

func NewBroker() *Broker { return &Broker{subs: map[chan string]struct{}{}} }

// Subscribe liefert einen Kanal mit Themen und eine Abmeldefunktion. Der Kanal
// wird geschlossen, wenn der Broker beendet wird.
func (b *Broker) Subscribe() (<-chan string, func()) {
	ch := make(chan string, 16)
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		close(ch)
		return ch, func() {}
	}
	b.subs[ch] = struct{}{}
	return ch, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if _, ok := b.subs[ch]; ok {
			delete(b.subs, ch)
			close(ch)
		}
	}
}

func (b *Broker) Publish(topic string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- topic:
		default: // Puffer voll – verwerfen
		}
	}
}

// Close beendet alle Abonnements (z. B. beim Herunterfahren).
func (b *Broker) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	for ch := range b.subs {
		delete(b.subs, ch)
		close(ch)
	}
}

// Subscribers liefert die Anzahl aktiver Abonnenten.
func (b *Broker) Subscribers() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.subs)
}
