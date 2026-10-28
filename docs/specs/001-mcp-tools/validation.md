# MCP 0.2.0 Validation

This is the initial implementation-stage snapshot. The subsequently authorized release-stage validation is recorded in `release-validation.md`; historical no-push/no-publication statements below describe the initial stage.

## Source And Scope

- Base: `9d3fbea`; DBX contract reference: `8c2bbe35b`.
- Active manifest, sidecar initialize metadata, npm package and lockfile: `0.2.0`.
- 15 exact declared/discovered tools, all `read`; tool inventory is in `plan.md` and the manifest.
- Existing workbench UI and original service endpoints remain unchanged.

## Tests And Builds

- Before implementation, existing `go test ./...` passed. New `TestMCP*` tests failed on missing `mcp/tools` and `mcp/call`, then passed after implementation.
- Additional observed RED/GREEN: retain nonsecret YAML configuration while redacting credentials, URL userinfo/query secrets, and oversized output.
- Backend `go test ./...`, `go test -race ./...`, `go vet ./...`: PASS.
- Vendored SDK `go test ./...`, `go test -race ./...`, `go vet ./...`: PASS.
- `npm test`: 6 files / 20 tests PASS; `npm run build`: vue-tsc and Vite PASS.

- Source and packaged manifests validate against the complete DBX upstream JSON Schema draft 2020-12.
- Six unsigned candidate targets built: macOS arm64/x64, Windows arm64/x64, Linux arm64/x64. ZIP contents/checksum metadata, embedded binary architecture and packaged MCP declarations independently checked.
- Native macOS arm64 sidecar initialize identity/protocol/version and stdio `mcp/tools` discovery tested; malformed/unknown tool call reports failure without creating persistent state.
- `git diff --check`: PASS.

## Behavior And Security

- Discovery is local metadata only: no HTTP/login/session cache effects.
- Calls require host lifecycle identity; schemas reject unknown credentials/routing/operation arguments before service I/O.
- Request-local clients honor existing connection and routing checks, leave interactive sessions unchanged, and close owned idle transports where owned by the client.
- Local HTTP fixtures exercise all 15 operation routes, invalid arguments/identity, current authentication, readonly usage, result/error envelopes and secret masking.
- MCP outputs have a 1 MiB cap. Structured credential fields, known lifecycle secrets and URL credentials are redacted. Parsed YAML authentication sections are withheld while nonsecret settings remain visible. This is not a secret-bearing backup export.

## Limits

- No real service, user credentials, saved DBX connections, grants or production writes used.
- No UI installation or real external DBX host/grant/confirmation acceptance; no runtime testing on Windows/Linux or macOS x64.
- Candidates remain unsigned local review artifacts, not signed Store releases. No commit, push, publication, installation, DBX restart or grant change performed.
- Generic strings can contain application-defined sensitive content not identifiable as a credential; redaction is not a universal data-loss-prevention guarantee.
- Exact current tool declarations need matching updated DBX and standalone MCP builds; the existing engine range is intentionally retained rather than guessing a release version for the upstream contract.

## Final Hardening

- Explicit nulls and noncanonical/zero pagination arguments were observed incorrectly defaulting before new tests; strict per-tool validation now rejects them before service I/O. Final tests/race/vet and six regenerated candidates cover the updated source.
