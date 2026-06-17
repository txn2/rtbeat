# CLAUDE.md

Guidance for Claude Code (and other agents) working in this repository.

## Project Overview

**rtbeat** is an [Elastic Beat](https://www.elastic.co/beats/) built on **libbeat v7.17.29**. It runs
an HTTP server (default `:8081`) that:

- accepts `POST /in` with an rxtx `MessageBatch` JSON body, republishing each message as a Beats event
  (original payload under `rxtxMsg`, plus `clientIp` and `type`) into the configured libbeat output
  (Elasticsearch / Logstash / Kafka / Redis / file);
- exposes Prometheus metrics at `GET /metrics`.

It is a small codebase: `main.go` (entrypoint) → `cmd/root.go` (libbeat root command) →
`beater/rtbeat.go` (the beat: HTTP server, publish loop, metrics) → `config/config.go` (port/timeout).

## Build, Test, Lint

```bash
make verify   # what CI runs: check-go-version + tidy-check + lint + test + validate-actions
make build    # CGO_ENABLED=0 go build -o rtbeat .
make test     # go test -race -coverprofile=coverage.txt -covermode=atomic ./...
make lint     # golangci-lint v2.12.1 (auto-installed into .tools/)
```

- Go **1.26**, `CGO_ENABLED=0` (static binaries; no cgo).
- Lint must be clean (`.golangci.yml`, golangci-lint v2).
- GitHub Action references are pinned to commit SHAs; `scripts/validate-action-shas.sh` enforces it.

## Dependency Constraints (read before touching go.mod)

This module depends on Elastic libbeat, which is unusually fragile under Go modules:

1. **Mirror replaces.** libbeat's own `go.mod` uses `replace` directives. Go modules do **not** apply a
   dependency's replaces transitively, so they are copied into this `go.mod` (sarama, fsnotify,
   gopacket, goja, go-winio). Keep them in sync with `github.com/elastic/beats/v7@v7.17.29` if the beats
   version ever changes.
2. **Pinned legacy transitive deps.** `txn2/rxtx` is pinned to the production revision (`v1.3.2`) to
   preserve the `rtq.MessageBatch` wire format. Its 2018-era deps `github.com/coreos/bbolt` and
   `github.com/satori/go.uuid` are pinned to specific commits because newer tags renamed the bbolt
   package and changed `uuid.NewV4`'s signature, which breaks rtq's source. **Do not bump these.**
3. `make tidy-check` fails the build if `go.mod`/`go.sum` drift.

When changing dependencies, run `make verify` and confirm the binary still reports
`rtbeat version 7.17.29` and republishes events unchanged.

## libbeat v7.17 API notes

These differ from the pre-modules alpha the project was originally written against:

- ACK callback: `beat.ClientConfig.ACKHandler = acker.RawCounting(func(int){...})`
  (the old `ACKCount` field was removed).
- Root command: `cmd.GenRootCmdWithSettings(beater.New, instance.Settings{Name: Name})`
  (old `cmd.GenRootCmd` removed).
- Events use `common.MapStr` and `beat.Event`.

## Release

Tag `v*` → GitHub Actions runs GoReleaser v2: signed (Cosign keyless) archives with SBOMs, SLSA
provenance, multi-arch container images (`ghcr.io/txn2/rtbeat`), and a Homebrew formula. Releases are created as
drafts.
