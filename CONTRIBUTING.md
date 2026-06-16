# Contributing to rtbeat

Thank you for your interest in contributing to rtbeat!

rtbeat is an [Elastic Beat](https://www.elastic.co/beats) built on
[libbeat](https://github.com/elastic/beats) v7.17. It runs an HTTP server that
accepts [rxtx](https://github.com/txn2/rxtx) `MessageBatch` payloads on
`POST /in`, parses each message, and republishes them as Beats events to the
configured Elastic output (Elasticsearch or Logstash). It also exposes
Prometheus metrics on `/metrics`. Behavior is configured through `rtbeat.yml`.

## Pull Request Policy

We welcome pull requests for:

- **Bug fixes** — fixing issues and errors
- **Tests** — improving test coverage
- **Documentation** — clarifying usage, fixing typos, adding examples
- **Stability and security improvements** — performance, error handling,
  dependency hygiene, and supply-chain hardening

For non-trivial changes, please open an issue first to discuss the approach.

## Development Setup

### Prerequisites

- **Go 1.26** (the toolchain CI uses; `make verify` will refuse to run on a
  mismatched major/minor version)
- `git`, `make`, and `curl`
- Optional for end-to-end testing: a reachable Elasticsearch or Logstash
  instance and an rxtx source to send batches

### Building

```bash
# Clone the repository
git clone https://github.com/txn2/rtbeat.git
cd rtbeat

# Build with make (CGO_ENABLED=0, output ./rtbeat)
make build

# Or build directly
CGO_ENABLED=0 go build -o rtbeat .
```

rtbeat builds as a static binary (`CGO_ENABLED=0`).

### Running Locally

```bash
# Run against the sample configuration. By default rtbeat listens on :8081
# and accepts POST /in; metrics are served on /metrics.
./rtbeat -c rtbeat.yml -e

# Send a test batch (rxtx MessageBatch JSON) to the ingest endpoint
curl -XPOST http://localhost:8081/in -d @testdata/batch.json

# Check Prometheus metrics
curl http://localhost:8081/metrics
```

The `-e` flag logs to stderr; `-c` selects the config file. See
`rtbeat.reference.yml` for the full set of configurable options.

## Verify Before You Push

Run `make verify` before opening or updating a pull request. It runs the same
checks the CI pipeline does, so you find problems locally instead of in a red
PR:

```bash
make verify
```

`make verify` performs, in order:

1. **check-go-version** — confirms your local Go matches the required 1.26.x
2. **tidy-check** — fails if `go.mod` / `go.sum` are not tidy
3. **lint** — `golangci-lint` (auto-installed at the CI-pinned version into
   `.tools/`)
4. **test** — `go test -race -coverprofile=... -covermode=atomic ./...`
5. **validate-actions** — confirms GitHub Action references are pinned to SHAs

Individual targets are available too: `make lint`, `make test`, `make build`,
`make tidy`, `make tidy-check`, `make validate-actions`. Run `make help` for the
full list.

## Running Tests

```bash
# Full suite under the race detector with coverage (matches CI)
make test

# Or run go test directly
go test ./...
go test -v ./...
go test -race ./...

# View a coverage report
go test -coverprofile=coverage.txt ./...
go tool cover -html=coverage.txt
```

Prefer table-driven tests, and add tests alongside bug fixes and new behavior.

## Dependency Constraints (read before touching go.mod)

rtbeat depends on `github.com/elastic/beats/v7@v7.17.29`. This pin is
deliberate and the surrounding module configuration is fragile. Please do not
bump these casually.

- **Mirrored `replace` directives.** Go does not apply a dependency's `replace`
  directives transitively, so `go.mod` mirrors the `replace` block from
  `elastic/beats` v7.17.29 (e.g. `Microsoft/go-winio`, `Shopify/sarama`,
  `dop251/goja`, `fsnotify/fsnotify`, `google/gopacket`). These exist so the
  build resolves the same forks libbeat itself uses. Removing or changing them
  will break compilation. If you upgrade libbeat, re-derive this block from the
  matching `elastic/beats` `go.mod`.
- **Pinned commit-level dependencies.** `github.com/coreos/bbolt` and
  `github.com/satori/go.uuid` are pinned to specific commits/pseudo-versions
  required by the libbeat tree. Do not let `go mod tidy` or an IDE "upgrade"
  silently move these — `make verify` will fail the `tidy-check` if they drift.
- **Upgrading libbeat is a focused change.** Bumping `elastic/beats/v7` should
  be its own PR. Update the version, re-sync the `replace` block and pinned
  commits to match upstream, run `make verify`, and confirm rtbeat still starts
  and publishes events end-to-end.

When you intentionally add or update a dependency, run `make tidy` and commit
the resulting `go.mod` / `go.sum` changes.

## Code Style

### Go Conventions

- Follow standard Go formatting (`gofmt` / `go fmt ./...`)
- Run `go vet ./...` and `golangci-lint run` (via `make lint`) and resolve
  findings before pushing
- Add doc comments for exported functions and types
- Handle errors explicitly; do not discard them silently

### Project Conventions

- Use `go.uber.org/zap` for structured logging, consistent with the existing
  code
- Keep the HTTP handlers (`/in`, `/metrics`) and the Beats publishing path
  decoupled and well tested
- Follow existing patterns in the codebase rather than introducing new ones

## Making Changes

1. Fork the repository and create a branch:
   `git checkout -b fix/short-description`
2. Make your changes and add tests for new or fixed behavior
3. Run `make verify` and ensure it passes
4. Commit with clear, descriptive messages
5. Open a pull request against `master`

### Commit Messages and Sign-off

Use clear, descriptive commit messages that explain the what and the why:

```
Fix: reject malformed rxtx batches with 400 instead of panicking

- Validate MessageBatch before publishing
- Return a JSON error body for bad input
- Add a test covering a truncated payload
```

Sign off your commits to certify the [Developer Certificate of Origin](https://developercertificate.org/):

```bash
git commit -s -m "Fix: reject malformed rxtx batches with 400"
```

The `-s` flag adds a `Signed-off-by` trailer with your name and email.

### Pull Request Guidelines

1. **Title** — a clear description of the change
2. **Description** — explain what and why, not just what
3. **Tests** — include tests for bug fixes and new behavior
4. **Documentation** — update `README.md`, `rtbeat.reference.yml`, or other
   docs if behavior or configuration changes
5. **Size** — keep PRs focused; split large changes
6. **Checks** — confirm `make verify` passes locally

## Reporting Issues

### Bug Reports

Include:

- rtbeat version (`rtbeat version`)
- The relevant portion of your `rtbeat.yml` (redact secrets)
- The Elastic output in use (Elasticsearch or Logstash) and its version
- A sample rxtx batch or request that triggers the problem, if applicable
- Operating system and version
- Steps to reproduce, and expected vs. actual behavior
- Relevant log output

### Security Issues

For security vulnerabilities, please follow [SECURITY.md](SECURITY.md) instead
of opening a public issue.

## Code of Conduct

Please review our [Code of Conduct](CODE_OF_CONDUCT.md) before contributing.

## Questions?

Open a [GitHub issue](https://github.com/txn2/rtbeat/issues) for questions or
discussion.

## License

By contributing, you agree that your contributions will be licensed under the
Apache License 2.0.
