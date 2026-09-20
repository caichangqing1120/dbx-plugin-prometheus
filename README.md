# Prometheus for DBX

An independent, read-only DBX workbench for the Prometheus HTTP API. The plugin queries one Prometheus server; it does not connect to Alertmanager or change Prometheus configuration.

## Connect

Requires DBX 0.6.14+ and Host API 1. Enter the host, port, HTTP/HTTPS scheme and optional reverse-proxy context path. Basic Auth username and password are optional but must be entered together. The password is handled by DBX's password field; it is not written to the plugin source or logs. Prefer HTTPS for Basic Auth. A DBX SSH/proxy/HTTP tunnel is used when configured; if the host does not provide a tunnel endpoint, the plugin fails closed rather than connecting directly.

The connection check reads `/api/v1/status/buildinfo`. The workbench offers:

- Instant and range PromQL queries; ranges are limited to 11,000 points, responses to 4 MiB and displayed series to 100.
- Active scrape targets, health and last errors (`/api/v1/targets`).
- Active alerts (`/api/v1/alerts`) and recording/alerting rule groups (`/api/v1/rules`).

All requests use GET and fixed endpoint paths; redirects are rejected. A query is executed against the configured Prometheus server, so restrict connection access and query size as appropriate to your environment. There are no native listeners, child processes or persistent plugin files.

## Build

Requires Node.js 22+, Go 1.22+ and `@dbx-app/plugin-cli@0.1.9`.

```sh
npm ci --ignore-scripts
npm test
npm run build
go -C backend test -race ./...
go -C backend vet ./...
npm run package:all
go run scripts/verify-package.go dist/io.github.caichangqing1120.prometheus-0.1.0-darwin-arm64.dbxp --handshake
```

The packaging script produces six unsigned macOS, Windows and Linux ARM64/x64 candidates, checksums, per-target metadata and `dist/release-candidates.json`. Cross-built packages are checked for architecture and checksum but have not been run in DBX on each OS. Backend tests use an `httptest` fixture, and UI checks use synthetic data; no live Prometheus deployment has been accepted.

## Marketplace

The source and unsigned release candidates are submitted to `t8y2/dbx-store` through a candidate PR. DBX maintainers review, sign and merge the exact bytes before the plugin appears in the official catalog. A public repository, tag or candidate PR alone is not an official listing. No signing key belongs in this repository.

The plugin source is MIT-licensed. See `LICENSE`, `assets/THIRD_PARTY_NOTICES.txt` and `backend/sdk/PROVENANCE.md` for license and SDK provenance. Prometheus is a trademark of The Linux Foundation; this independent integration is not affiliated with the Prometheus project.
