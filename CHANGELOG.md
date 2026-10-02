# Changelog

All notable changes to this project are documented in this file.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.3.0] – 2026-10-02

### Added
- Installable as an app (PWA): web app manifest, icons and a service worker. The interface
  also opens offline; API data is never cached.
- Blocklist check: the public IPv4 is checked against DNS blocklists (Spamhaus ZEN, SpamCop,
  PSBL, Mailspike) after every IP change and every 24 hours. The result is shown in the IP
  card; new listings trigger the new webhook event `blocklist_listed`. Spamhaus PBL is shown as
  information only. `DNSBL_LISTS` selects the lists or disables the check (`off`).
- Reachability and certificate monitoring: new page **Reachability** checks your own services
  over HTTP(S) – per record with a switch in the record dialog, or any URL. Outages are reported
  after two failures in a row; TLS certificates are verified (expiry, host name, issuer) with a
  warning before they expire (default 14 days). New webhook events `site_down`,
  `site_recovered` and `cert_expiring`; check interval and warning period under **Settings**.

## [0.2.2] – 2026-10-01

### Added
- The dashboard shows the internet provider of each public address: AS number and name,
  country, network and reverse DNS name. Looked up via DNS at Team Cymru (no API key);
  `ISP_LOOKUP=off` disables it.

## [0.2.1] – 2026-09-30

### Fixed
- The sidebar (live status, language, theme, sign out) stays visible on long pages instead
  of scrolling away.
- Update log and record messages written before 0.2.0 are now translated as well: known
  German texts are converted once on start.

## [0.2.0] – 2026-09-30

### Added
- English web interface: the language follows the browser and can be switched (EN | DE);
  server messages (errors, record status, update log) are translated as well.
- Setting for the language of notifications (default German).
- DNS propagation status per record: public resolvers and the zone's authoritative name
  servers are checked automatically after every change and on demand. Configurable with
  `DNSCHECK_RESOLVERS` and `DNSCHECK_AUTHORITATIVE`.
- Light mode: the appearance follows the system setting or can be set to light or dark.

### Changed
- Docker Compose: the image version is pinned via `DNSDECK_VERSION` in `.env` (required,
  no `latest`); host port (`DNSDECK_PORT`) and time zone (`TZ`, default `UTC`) are configurable.
- Documentation in English and German, MIT license.


## [0.1.0] – 2026-09-30

First public release.

### Added
- Public IPv4/IPv6 detection from several sources with majority vote and plausibility checks.
- Dynamic DNS for Cloudflare A and AAAA records: only updates on differences, creates missing
  records, adopts existing ones with their proxy/TTL settings.
- Cloudflare Tunnel monitoring with status history and 24 h / 7 day uptime.
- Live web interface (Server-Sent Events): dashboard, records, tunnels, history, settings.
- Configurable webhooks with templates for ntfy, Gotify, Discord, Slack, Telegram and
  Home Assistant; secrets referenced from environment variables.
- Single-container deployment (distroless, non-root, amd64 and arm64) published to ghcr.io.

[Unreleased]: https://github.com/mrcdlm/dnsdeck/compare/v0.3.0...HEAD
[0.3.0]: https://github.com/mrcdlm/dnsdeck/compare/v0.2.2...v0.3.0
[0.2.2]: https://github.com/mrcdlm/dnsdeck/compare/v0.2.1...v0.2.2
[0.2.1]: https://github.com/mrcdlm/dnsdeck/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/mrcdlm/dnsdeck/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/mrcdlm/dnsdeck/releases/tag/v0.1.0
