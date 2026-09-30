# Changelog

All notable changes to this project are documented in this file.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

[Unreleased]: https://github.com/mrcdlm/dnsdeck/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/mrcdlm/dnsdeck/releases/tag/v0.1.0
