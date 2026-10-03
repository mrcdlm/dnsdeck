# Contributing to dnsdeck

Thanks for your interest! Bug reports, ideas and pull requests are welcome.

## Reporting bugs and requesting features

Use the [issue templates](https://github.com/mrcdlm/dnsdeck/issues/new/choose). Please include
the dnsdeck version (shown under **Settings → System**) and relevant log lines
(`docker compose logs dnsdeck`).

**Never post secrets** – API tokens, `APP_PASSWORD`, webhook URLs or tokens. Security problems
are reported privately as described in [SECURITY.md](SECURITY.md).

## Development

See [Development](README.md#development) in the README for setup, the Cloudflare API mock and
how to run the checks. In short:

```sh
go vet ./... && go test ./...
cd web && npm run lint && npm run build
cd e2e && npm ci && npx playwright install chromium && npx playwright test
```

## Pull requests

- Keep changes focused; open an issue first for larger features so we can agree on the approach.
- Add or update tests – the Cloudflare API is always mocked, tests never touch real accounts.
- Every user-facing text exists in English and German: server messages in
  `internal/i18n/catalog.go`, interface texts in `web/src/locales/{en,de}.json`
  (`npm run lint` checks that both locales have the same keys).
- Secrets only ever come from environment variables – never from the database, logs or API
  responses.
- Add an entry under `[Unreleased]` in [CHANGELOG.md](CHANGELOG.md) for user-visible changes.
- Notifications stay generic webhooks; services such as ntfy or Telegram are only presets in
  the frontend, not separate code paths.

By contributing you agree that your contribution is licensed under the [MIT license](LICENSE).
