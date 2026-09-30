# Changelog

All notable changes to this project are documented in this file.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

### Notes
- Update log entries written by 0.1.0 keep their original German text.

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

[Unreleased]: https://github.com/mrcdlm/dnsdeck/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/mrcdlm/dnsdeck/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/mrcdlm/dnsdeck/releases/tag/v0.1.0
