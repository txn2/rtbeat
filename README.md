[![rxtx data transmission](https://raw.githubusercontent.com/txn2/rtbeat/master/mast-logo.jpg)](https://github.com/txn2/rtbeat)
[![Release](https://img.shields.io/github/release/txn2/rtbeat.svg)](https://github.com/txn2/rtbeat/releases)
[![CI](https://github.com/txn2/rtbeat/actions/workflows/ci.yml/badge.svg)](https://github.com/txn2/rtbeat/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/txn2/rtbeat/graph/badge.svg)](https://codecov.io/gh/txn2/rtbeat)
[![Go Report Card](https://goreportcard.com/badge/github.com/txn2/rtbeat)](https://goreportcard.com/report/github.com/txn2/rtbeat)
[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/txn2/rtbeat/badge)](https://securityscorecards.dev/viewer/?uri=github.com/txn2/rtbeat)
[![ghcr.io](https://img.shields.io/badge/ghcr.io-txn2%2Frtbeat-2496ED?logo=github)](https://github.com/txn2/rtbeat/pkgs/container/rtbeat)

# Rtbeat

[Rtbeat](https://github.com/txn2/rtbeat) is an [Elastic Beat](https://www.elastic.co/beats/) that
receives HTTP POST data from [rxtx](https://github.com/txn2/rxtx) and publishes each message as a
Beats event into [Elasticsearch], [Logstash], [Kafka], [Redis], or directly to log files.

It runs a small HTTP server (default port `8081`):

- `POST /in` — accepts an rxtx `MessageBatch` JSON body; every message in the batch is republished
  as a Beats event with the original payload under `rxtxMsg`, plus `clientIp` and `type`.
- `GET /metrics` — Prometheus metrics (`rtbeat_batches_received`, `rtbeat_messages_parsed`,
  `rtbeat_acks_received`, `rtbeat_current_acks`).

## Requirements

- [Go](https://go.dev/dl/) 1.26 or greater (module-based; no GOPATH layout required).

## Build

```bash
make build          # CGO_ENABLED=0 go build -o rtbeat .
# or
go build -o rtbeat .
```

## Run

```bash
./rtbeat -c rtbeat.yml -e -d "*"
```

`rtbeat.yml` configures the listen port and the Elastic output. See `rtbeat.reference.yml` for the
full set of libbeat output and processor options.

## Docker

```bash
docker run --rm -p 8081:8081 -v "$PWD/rtbeat.yml:/rtbeat.yml" \
  ghcr.io/txn2/rtbeat -c /rtbeat.yml -e
```

## Develop

Run the same checks CI runs before pushing:

```bash
make verify   # check-go-version + tidy-check + lint + test + validate-actions
```

Individual targets: `make lint`, `make test`, `make build`, `make tidy`. See
[CONTRIBUTING.md](CONTRIBUTING.md).

### Dependency notes

rtbeat is built on Elastic **libbeat v7.17.29**. libbeat relies on a set of `replace` directives in
its own `go.mod`; because Go modules do not apply a dependency's replaces transitively, those are
mirrored into this module's `go.mod`. The `txn2/rxtx` message type and its 2018-era transitive
dependencies (`coreos/bbolt`, `satori/go.uuid`) are pinned to specific commits so the wire format and
compilation stay stable. Do not bump these casually — `make tidy-check` will flag drift.

## Releasing

Releases are built by [GoReleaser](https://goreleaser.com/) on tag push (`v*`) via GitHub Actions,
producing signed (Cosign, keyless) archives with SBOMs and SLSA provenance, plus multi-arch Docker
images. To cut a release:

```bash
git tag -a v1.2.3 -m "Version 1.2.3"
git push origin v1.2.3
```

## Resources

- [Elasticsearch] · [Logstash] · [Kafka] · [Redis]
- [rxtx](https://github.com/txn2/rxtx) — the data transmission client that posts to rtbeat
- [GoReleaser](https://goreleaser.com/) · [Docker](https://www.docker.com/)

## License

Apache 2.0 — see [LICENSE](LICENSE).

[Elasticsearch]: https://www.elastic.co/
[Logstash]: https://www.elastic.co/products/logstash
[Kafka]: https://kafka.apache.org/
[Redis]: https://redis.io/
