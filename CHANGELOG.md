# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Fixed

- **Durability:** `POST /in` now acknowledges a batch only after the output confirms delivery
  (`GuaranteedSend` + per-batch ack), returning 200 on delivery or 504 on timeout (configurable via
  `timeout`), so an upstream sender keeps its durable copy until the event is genuinely downstream.
  Graceful shutdown now drains in-flight events (configurable via `shutdown_timeout`) before closing
  the publisher, and stops HTTP intake before draining. Removed the stray leading empty event that
  was published with every batch. (#1)

### Added

- End-to-end durability tests (`make e2e` / `go test -tags e2e ./test/e2e/...`): they run the real
  rtbeat binary against a controllable lumberjack (logstash) server and assert 200-only-after-real-ack,
  504-on-stall, and in-flight drain on SIGTERM. Wired into CI as a dedicated job.

### Changed

- Migrated from glide/GOPATH vendoring to Go modules (`go.mod`), targeting Go 1.26.
- Upgraded Elastic libbeat to v7.17.29 (from the v7.0.0-alpha line) and refreshed all direct
  dependencies (gin, prometheus client, zap).
- Replaced the libbeat-generated Makefile and Travis CI with a `make verify` workflow and GitHub
  Actions (CI, CodeQL, OpenSSF Scorecard, Dependabot, docs).
- Reworked the GoReleaser pipeline to v2: static `CGO_ENABLED=0` builds for linux/darwin
  (amd64, arm64), Cosign keyless signing, SBOMs, SLSA provenance, and multi-arch Docker images.

### Added

- `golangci-lint` v2 configuration and a clean lint baseline.
- `SECURITY.md`, `CODE_OF_CONDUCT.md`, `CONTRIBUTING.md`, issue/PR templates, `CODEOWNERS`,
  `codecov.yml`, and MkDocs documentation.

### Notes

- The rxtx `MessageBatch` wire format is unchanged; `txn2/rxtx` is pinned to the prior production
  revision and its 2018-era transitive dependencies (`coreos/bbolt`, `satori/go.uuid`) are pinned to
  compatible commits.
