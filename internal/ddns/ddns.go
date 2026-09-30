// Package ddns gleicht die verwalteten DNS-Einträge mit der aktuellen
// öffentlichen IP ab.
package ddns

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/mrcdlm/dnsdeck/internal/events"
	"github.com/mrcdlm/dnsdeck/internal/i18n"
	"github.com/mrcdlm/dnsdeck/internal/ipdetect"
	"github.com/mrcdlm/dnsdeck/internal/notify"
	"github.com/mrcdlm/dnsdeck/internal/providers"
	"github.com/mrcdlm/dnsdeck/internal/store"
)

// IPs sind die aktuell bekannten öffentlichen Adressen ("" = unbekannt).
type IPs struct {
	V4 string
	V6 string
}

func IPsFromState(s ipdetect.State) IPs { return IPs{V4: s.IPv4.IP, V6: s.IPv6.IP} }

func (ips IPs) forType(recordType string) string {
	if recordType == "AAAA" {
		return ips.V6
	}
	return ips.V4
}

type recordStore interface {
	ListRecords(ctx context.Context) ([]store.Record, error)
	GetRecord(ctx context.Context, id int64) (store.Record, error)
	SetRecordSyncState(ctx context.Context, id int64, st store.RecordSyncState) error
	InsertUpdateLog(ctx context.Context, e store.UpdateLogEntry) (int64, error)
}

// Updater führt den Abgleich aus. Abgleiche laufen nacheinander, damit
// Scheduler und manuelle Auslöser sich nicht überschneiden.
type Updater struct {
	store    recordStore
	provider providers.Provider // nil = nicht konfiguriert
	log      *slog.Logger
	now      func() time.Time
	mu       sync.Mutex
	// Pub wird nach jedem Abgleich informiert (optional).
	Pub events.Publisher
	// Notifier erhält Fehler und Erholungen (optional).
	Notifier notify.Notifier
	// OnChange wird nach einem Abgleich aufgerufen, dessen Ergebnis sich im
	// DNS verbreiten muss – z. B. zum Start der Verbreitungsprüfung (optional,
	// darf nicht blockieren).
	OnChange func(id int64)
}

func NewUpdater(st recordStore, p providers.Provider, log *slog.Logger) *Updater {
	return &Updater{store: st, provider: p, log: log, now: time.Now}
}

// SyncAll gleicht alle Einträge ab. Fehler einzelner Einträge landen im
// Record-Status und im Update-Log; zurückgegeben werden nur DB-Fehler.
func (u *Updater) SyncAll(ctx context.Context, ips IPs, trigger string) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	defer events.Publish(u.Pub, events.TopicRecords)
	recs, err := u.store.ListRecords(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for _, r := range recs {
		if err := u.sync(ctx, r, ips, trigger); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// SyncRecord gleicht einen einzelnen Eintrag ab und liefert dessen neuen Stand.
func (u *Updater) SyncRecord(ctx context.Context, id int64, ips IPs, trigger string) (store.Record, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	defer events.Publish(u.Pub, events.TopicRecords)
	r, err := u.store.GetRecord(ctx, id)
	if err != nil {
		return store.Record{}, err
	}
	if err := u.sync(ctx, r, ips, trigger); err != nil {
		return store.Record{}, err
	}
	return u.store.GetRecord(ctx, id)
}

func (u *Updater) sync(ctx context.Context, r store.Record, ips IPs, trigger string) error {
	now := u.now()

	if !r.Enabled {
		if r.Status == store.RecordPaused {
			return nil
		}
		return u.store.SetRecordSyncState(ctx, r.ID, store.RecordSyncState{Status: store.RecordPaused, CheckedAt: now})
	}

	ip := ips.forType(r.Type)
	if ip == "" {
		fam := "IPv4"
		if r.Type == "AAAA" {
			fam = "IPv6"
		}
		return u.store.SetRecordSyncState(ctx, r.ID, store.RecordSyncState{
			Status: store.RecordSkipped, Message: i18n.M("ddns.no_ip", "family", fam), CheckedAt: now})
	}

	if u.provider == nil {
		return u.fail(ctx, r, ip, trigger, now, &i18n.Error{Msg: i18n.M("cf.not_configured"), Wrap: providers.ErrNotConfigured})
	}

	// Ist-Zustand beim Anbieter abfragen – geändert wird nur bei Abweichung.
	cur, err := u.provider.GetRecord(ctx, r.ZoneID, r.Name, r.Type)
	if errors.Is(err, providers.ErrNotFound) {
		created, err := u.provider.UpdateRecord(ctx, providers.Record{
			Zone: r.ZoneID, Name: r.Name, Type: r.Type, Content: ip, TTL: r.TTL, Proxied: r.Proxied})
		if err != nil {
			return u.fail(ctx, r, ip, trigger, now, err)
		}
		u.log.Info("DNS-Eintrag angelegt", "record", r.Name, "type", r.Type, "ip", ip)
		u.writeLog(ctx, r, trigger, store.ResultCreated, "", ip, i18n.M("ddns.created"), now)
		u.recovered(ctx, r, trigger, ip, now)
		return u.save(ctx, r.ID, synced(created, now, true), true)
	}
	if err != nil {
		return u.fail(ctx, r, ip, trigger, now, err)
	}

	// Proxy/TTL: Cloudflare hat das letzte Wort. dnsdeck überträgt sie nur,
	// wenn sie in dnsdeck geändert wurden (SettingsPending); sonst werden die
	// Werte von Cloudflare übernommen.
	want := cur
	want.Content = ip
	if r.SettingsPending {
		want.Proxied, want.TTL = r.Proxied, r.TTL
	}
	changes := settingChanges(cur, want)
	ipDiffers := cur.Content != ip
	adopting := r.ProviderRecordID == "" // erstmals mit diesem Eintrag verknüpft

	if !ipDiffers && len(changes) == 0 {
		if adopting {
			msg := i18n.M("ddns.adopted", "proxy", onOff(cur.Proxied), "ttl", FormatTTL(cur.TTL))
			u.log.Info("DNS-Eintrag übernommen", "record", r.Name, "type", r.Type, "proxied", cur.Proxied, "ttl", cur.TTL)
			u.writeLog(ctx, r, trigger, store.ResultAdopted, cur.Content, cur.Content, msg, now)
		} else if !r.SettingsPending && (cur.Proxied != r.Proxied || (!cur.Proxied && cur.TTL != r.TTL)) {
			u.log.Info("Proxy/TTL von Cloudflare übernommen", "record", r.Name, "type", r.Type,
				"proxied", cur.Proxied, "ttl", cur.TTL)
		}
		u.recovered(ctx, r, trigger, cur.Content, now)
		// Prüfen, wenn noch nie geprüft oder der Proxy bei Cloudflare umgestellt wurde
		check := adopting || len(r.Propagation) == 0 || cur.Proxied != r.Proxied
		return u.save(ctx, r.ID, synced(cur, now, false), check)
	}

	upd, err := u.provider.UpdateRecord(ctx, want)
	if err != nil {
		return u.fail(ctx, r, ip, trigger, now, err)
	}
	if adopting {
		changes = append([]i18n.Msg{i18n.M("ddns.adopted_short")}, changes...)
	}
	msg := i18n.JoinMsgs(changes...)
	u.log.Info("DNS-Eintrag aktualisiert", "record", r.Name, "type", r.Type,
		"old", cur.Content, "new", ip, "changes", i18n.T(i18n.EN, msg))
	u.writeLog(ctx, r, trigger, store.ResultUpdated, cur.Content, ip, msg, now)
	u.recovered(ctx, r, trigger, ip, now)
	return u.save(ctx, r.ID, synced(upd, now, true), true)
}

// save speichert das Ergebnis eines erfolgreichen Abgleichs; check: danach
// OnChange aufrufen.
func (u *Updater) save(ctx context.Context, id int64, st store.RecordSyncState, check bool) error {
	if err := u.store.SetRecordSyncState(ctx, id, st); err != nil {
		return err
	}
	if check && u.OnChange != nil {
		u.OnChange(id)
	}
	return nil
}

// recovered protokolliert und meldet, dass ein zuvor fehlerhafter Record
// wieder erfolgreich abgeglichen wurde.
func (u *Updater) recovered(ctx context.Context, r store.Record, trigger, ip string, now time.Time) {
	if r.Status != store.RecordError {
		return
	}
	msg := i18n.M("ddns.recovered")
	if prev := previousMsg(r); !prev.IsZero() {
		msg = i18n.M("ddns.recovered_after", "detail", i18n.Nest(prev))
	}
	u.log.Info("DNS-Abgleich wieder erfolgreich", "record", r.Name, "type", r.Type)
	u.writeLog(ctx, r, trigger, store.ResultRecovered, ip, ip, msg, now)
	notify.Send(u.Notifier, notify.Event{
		Type: notify.EventUpdateRecovered, Priority: notify.PriorityDefault, Time: now,
		TitleMsg:   i18n.M("notify.update_recovered.title", "name", r.Name, "type", r.Type),
		MessageMsg: i18n.M("notify.update_recovered.message", "name", r.Name, "ip", ip),
		Data:       map[string]string{"record": r.Name, "type": r.Type, "ip": ip},
	})
}

// previousMsg liefert die gespeicherte Meldung eines Records (Alttext als Rohtext).
func previousMsg(r store.Record) i18n.Msg {
	if !r.MessageMsg.IsZero() {
		return r.MessageMsg
	}
	if r.Message != "" {
		return i18n.Raw(r.Message)
	}
	return i18n.Msg{}
}

// synced beschreibt einen erfolgreichen Abgleich mit dem Stand beim Anbieter.
func synced(p providers.Record, now time.Time, changed bool) store.RecordSyncState {
	proxied, ttl := p.Proxied, p.TTL
	return store.RecordSyncState{ProviderRecordID: p.ID, CurrentIP: p.Content, Status: store.RecordOK,
		CheckedAt: now, Changed: changed, Proxied: &proxied, TTL: &ttl}
}

// settingChanges beschreibt Proxy-/TTL-Unterschiede, z. B. "Proxy: aus → an".
// Bei proxied Einträgen setzt Cloudflare die TTL selbst; sie zählt dann nicht.
func settingChanges(cur, want providers.Record) []i18n.Msg {
	var out []i18n.Msg
	if cur.Proxied != want.Proxied {
		out = append(out, i18n.M("ddns.proxy_change", "from", onOff(cur.Proxied), "to", onOff(want.Proxied)))
	}
	if !want.Proxied && cur.TTL != want.TTL {
		out = append(out, i18n.M("ddns.ttl_change", "from", FormatTTL(cur.TTL), "to", FormatTTL(want.TTL)))
	}
	return out
}

// onOff liefert einen übersetzbaren Verweis auf "an"/"aus".
func onOff(b bool) string {
	if b {
		return i18n.Ref("state.on")
	}
	return i18n.Ref("state.off")
}

// FormatTTL formatiert eine TTL wie im Frontend (1 = automatisch).
func FormatTTL(ttl int) string {
	switch {
	case ttl == 1:
		return "Auto"
	case ttl%86400 == 0:
		return fmt.Sprintf("%d d", ttl/86400)
	case ttl%3600 == 0:
		return fmt.Sprintf("%d h", ttl/3600)
	case ttl%60 == 0:
		return fmt.Sprintf("%d min", ttl/60)
	}
	return fmt.Sprintf("%d s", ttl)
}

// fail speichert den Fehler am Record. Ins Update-Log kommt er nur, wenn er
// sich vom vorherigen unterscheidet – sonst füllt ein dauerhafter Fehler das
// Log alle paar Minuten mit demselben Eintrag.
func (u *Updater) fail(ctx context.Context, r store.Record, ip, trigger string, now time.Time, cause error) error {
	msg := i18n.FromError(cause)
	repeated := r.Status == store.RecordError && i18n.Equal(previousMsg(r), msg)
	if !repeated || trigger != store.TriggerScheduled {
		u.log.Warn("DNS-Update fehlgeschlagen", "record", r.Name, "type", r.Type, "err", cause)
		u.writeLog(ctx, r, trigger, store.ResultError, r.CurrentIP, ip, msg, now)
	}
	if !repeated {
		notify.Send(u.Notifier, notify.Event{
			Type: notify.EventUpdateFailed, Priority: notify.PriorityHigh, Time: now,
			TitleMsg:   i18n.M("notify.update_failed.title", "name", r.Name, "type", r.Type),
			MessageMsg: i18n.M("notify.update_failed.message", "ip", ip, "detail", i18n.Nest(msg)),
			// Data.error bewusst englisch: maschinenlesbar, unabhängig von der Sprache
			Data: map[string]string{"record": r.Name, "type": r.Type, "ip": ip, "error": cause.Error()},
		})
	}
	return u.store.SetRecordSyncState(ctx, r.ID, store.RecordSyncState{
		Status: store.RecordError, Message: msg, CheckedAt: now})
}

func (u *Updater) writeLog(ctx context.Context, r store.Record, trigger, result, oldIP, newIP string, msg i18n.Msg, now time.Time) {
	id := r.ID
	_, err := u.store.InsertUpdateLog(ctx, store.UpdateLogEntry{
		RecordID: &id, RecordName: r.Name, RecordType: r.Type, Trigger: trigger, Result: result,
		OldIP: oldIP, NewIP: newIP, MessageMsg: msg, CreatedAt: now})
	if err != nil {
		u.log.Error("Update-Log schreiben fehlgeschlagen", "err", err)
		return
	}
	events.Publish(u.Pub, events.TopicUpdates)
}

// Service bündelt IP-Prüfung und Abgleich zu einem Durchlauf.
type Service struct {
	Tracker *ipdetect.Tracker
	Updater *Updater
}

// RunCycle prüft die IP und gleicht danach alle Einträge ab.
func (s *Service) RunCycle(ctx context.Context, trigger string) (ipdetect.State, error) {
	state, ipErr := s.Tracker.Check(ctx)
	syncErr := s.Updater.SyncAll(ctx, IPsFromState(state), trigger)
	return state, errors.Join(ipErr, syncErr)
}

// SyncRecord gleicht einen Eintrag mit den zuletzt bekannten IPs ab.
func (s *Service) SyncRecord(ctx context.Context, id int64, trigger string) (store.Record, error) {
	return s.Updater.SyncRecord(ctx, id, IPsFromState(s.Tracker.State()), trigger)
}
