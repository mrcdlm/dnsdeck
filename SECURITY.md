# Security Policy

## Supported versions

Security fixes are provided for the latest release. Please update to the newest version
before reporting an issue.

## Reporting a vulnerability

Please **do not open a public issue** for security problems. Report them privately via
[GitHub Security Advisories](https://github.com/mrcdlm/dnsdeck/security/advisories/new)
("Report a vulnerability").

Include a description of the issue, the affected version and, if possible, steps to
reproduce. You will receive a response as soon as possible; fixes are published as a new
release together with an advisory.

## Deployment recommendations

- Do not expose the web interface directly to the internet; use a reverse proxy with TLS or
  a Cloudflare Tunnel, ideally with Cloudflare Access in front.
- Use a long, random `APP_PASSWORD` and an API token restricted to the zones and account
  dnsdeck should manage.
- Keep secrets in `.env` (permissions `600`) and never commit it.
