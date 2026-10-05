# Changelog

All notable changes to this project are documented in this file.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- `TRUSTED_PROXIES`: behind a reverse proxy or Cloudflare Tunnel, the real client address is
  taken from `CF-Connecting-IP` / `X-Forwarded-For` – for the login rate limit and logs. Before,
  failed logins of one client blocked everyone behind the same proxy. The headers are only
  trusted from the configured proxies.

### Security
- Cross-site request forgery: write requests to the API are only accepted from dnsdeck's own
  origin. Before, a page on a sibling subdomain of the same domain could, for example, create a
  webhook and thereby read webhook secrets, despite the `SameSite=Strict` cookie.
- Login rate limit: attempts are counted before the password check, so parallel requests can
  no longer exceed the limit of 10 attempts per minute.
- Changing `APP_PASSWORD` now signs out all existing sessions. After updating, everyone has to
  sign in once again.

## [0.4.0] – 2026-10-05

### Added
- Speed test: download, upload, ping and jitter measured against the nearest Cloudflare data
  center (`speed.cloudflare.com`), on demand or on a schedule (off by default, 1 h – 7 days).
  New page with history chart and table, dashboard tile, data center and location per result.

## [0.3.2] – 2026-10-03

### Changed
- Docker Compose: the container drops all Linux capabilities and runs with
  `no-new-privileges` (dnsdeck needs neither). If you use your own `docker-compose.yml`,
  add `cap_drop: [ALL]` and `security_opt: [no-new-privileges:true]` below `restart:`.
- A warning is logged on start if a plain-text `APP_PASSWORD` is shorter than 12 characters.
- Updated screenshots; contribution guide and issue templates.
- GitHub releases show the changes from this changelog instead of a generic text.

## [0.3.1] – 2026-10-03

### Fixed
- Connection errors to Cloudflare are shown as a short message (e.g. “Cloudflare did not
  respond in time”) instead of the raw Go error with the full request URL; entries already
  stored are shortened when displayed.
- Long messages in the update log, history, records and reachability checks wrap instead of
  overflowing their card.

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

[Unreleased]: https://github.com/mrcdlm/dnsdeck/compare/v0.4.0...HEAD
[0.4.0]: https://github.com/mrcdlm/dnsdeck/compare/v0.3.2...v0.4.0
[0.3.2]: https://github.com/mrcdlm/dnsdeck/compare/v0.3.1...v0.3.2
[0.3.1]: https://github.com/mrcdlm/dnsdeck/compare/v0.3.0...v0.3.1
[0.3.0]: https://github.com/mrcdlm/dnsdeck/compare/v0.2.2...v0.3.0
[0.2.2]: https://github.com/mrcdlm/dnsdeck/compare/v0.2.1...v0.2.2
[0.2.1]: https://github.com/mrcdlm/dnsdeck/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/mrcdlm/dnsdeck/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/mrcdlm/dnsdeck/releases/tag/v0.1.0
