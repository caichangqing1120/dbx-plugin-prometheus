# prometheus MCP Upgrade Implementation Plan

> Execute with the foundation handoff and Superpowers TDD. Keep all edits inside this isolated worktree; no commits, pushes or releases are implicit.

**Goal:** Expose all current service capabilities safely to the updated DBX MCP host and upgrade to 0.2.0.
**Architecture:** A dedicated `backend/mcp.go` tool registry and adapter calls existing service handlers, preserving their guards. The manifest declares the exact registry and maximum tool access. The UI and service API contracts are unchanged.
**Tech Stack:** Go sidecar; existing plugin-specific frontend and packaging tools.
**Spec:** `spec.md` in this package (user-request scope and existing upstream protocol).

## Global Constraints

All requirements in Spec apply. No remote writes, installation, restarts, grant mutations or credential use. Version 0.2.0; ID/publisher unchanged; manifest has at most 64 tool declarations. Keep detached HEAD and evidence.

## Review Focus

- A model selects a read-labelled wrapper then smuggles a mutation or alternate method/path.
- A stale writable session is reused for a current readonly lifecycle or a different connection identity.
- A sidecar starts during discovery and unexpectedly logs in or creates workloads.
- A generic form/body hides credentials or authorization-affecting values.
- Version bumps or packaging leave initialize metadata inconsistent with the shipped manifest.

## Task 1: Inventory And Bridge (backend/mcp.go, backend/mcp_test.go, backend/main.go)

- [x] Read existing dispatch tables, safety checks, tests and this Spec; record the full exact tool-name/method/access inventory here before implementation.
- [x] Run existing `go test ./...` in backend and applicable frontend baseline. Record unavailable dependencies/failures.
- [x] Write failing behavior tests exercising real dispatch through local HTTP fixtures, identity, readonly, confirmation, error envelopes, discovery parity and zero discovery effects. Run `go test ./...` and confirm expected missing-bridge failures.
- [x] Implement `mcp/tools` / `mcp/call` in backend/mcp.go and wire Handle in backend/main.go. Reuse existing APIs and guards; no arbitrary RPC forwarding. Include explicit portable parameter schemas. Record any existing capability that cannot safely be exposed.
- [x] Run focused tests, then `go test ./...`, `go test -race ./...`, and `go vet ./...`. Expected: all pass.

## Task 2: Declarations, Version And Delivery Evidence (manifest.json, active build metadata, README.md)

- [x] Test exact tool-name parity and max-access mapping between discovery and manifest; then add the mcp contribution and meaningful toolsets.
- [x] Bump active version metadata to 0.2.0 without rewriting historical evidence. Extend README with access levels and user enablement instructions.
- [x] Run the applicable complete frontend tests/build and existing package checks. Verify packaged manifest and sidecar initialize metadata agree.
- [x] Record commands, results, full inventory, limitations and risks in validation.md in this package. Mark completed steps in this plan. Keep the uncommitted diff for review.

## Tool Inventory

- `prometheus_info -> prometheus/info, read`
- `prometheus_targets -> prometheus/targets, read`
- `prometheus_alerts -> prometheus/alerts, read`
- `prometheus_rules -> prometheus/rules, read`
- `prometheus_status_runtime -> prometheus/status_runtime, read`
- `prometheus_status_tsdb -> prometheus/status_tsdb, read`
- `prometheus_status_flags -> prometheus/status_flags, read`
- `prometheus_status_config -> prometheus/status_config, read`
- `prometheus_service_discovery_services -> prometheus/service_discovery_services, read`
- `prometheus_service_discovery -> prometheus/service_discovery, read`
- `prometheus_metric_names -> prometheus/metric_names, read`
- `prometheus_label_names -> prometheus/label_names, read`
- `prometheus_label_values -> prometheus/label_values, read`
- `prometheus_query -> prometheus/query, read`
- `prometheus_query_range -> prometheus/query_range, read`

## Progress

- Baseline `go test ./...`: PASS.
- New MCP tests fail on unsupported mcp/tools and mcp/call, as expected (task-local RED evidence).

## Implementation Notes

- Request-local clients deliberately avoid a cache: all tools are read-only and no cross-call server state is required.
- Configuration is parsed by pinned yaml.v3 before redaction; MIT/Apache license notice is bundled. Unknown YAML is withheld.


## Store Compatibility Patch 0.2.1

**Spec:** `spec.md`, Store Compatibility Patch 0.2.1 (2026-10-29). Existing historical upgrade tasks remain intact. The user-approved corrective patch is executed only in the current isolated worktree; commits/publication are handled separately.

- [x] Confirm clean detached baseline and passing existing frontend/backend suites.
- [x] Add `scripts/test-release-contract.mjs`: run the active generator with compiler-only fixture subprocesses, import its unmodified JSON through actual Store sync, and independently test active listing metadata. Observe both failures before implementation.
- [x] Correct `scripts/package.mjs` author envelope and `.dbx-store.json` unsupported metadata; bump active versions and update version-aware tests/verifier.
- [x] Run focused regression GREEN; complete frontend, backend, SDK, race, vet and production build checks. Build/verify all six targets with existing scripts and handshake the native sidecar.
- [x] Import the real generated release without conversion using actual Store sync/validator, validate its schema and checksum/size parity; record patch-specific repo-relative notes and validation. No remote writes from this execution worker.

Review focus: flat author-vs-Store candidate confusion, unsupported metadata fields, altered MCP contributions, missing architecture assets, and stale initialize/package versions. The regression exercises the real importer/validator rather than reproducing the output builder's expectations. No execution helpers or legacy ledgers are used.
