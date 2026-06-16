# rtbeat

**rtbeat** is an [Elastic Beat](https://www.elastic.co/beats/) that receives HTTP POST data from
[rxtx](https://github.com/txn2/rxtx) and publishes each message as a Beats event into Elasticsearch,
Logstash, Kafka, Redis, or directly to log files.

## How it works

rtbeat runs a small HTTP server (default port `8081`):

- **`POST /in`** — accepts an rxtx `MessageBatch` JSON body. Each message in the batch is republished
  as a Beats event, with the original message under the `rxtxMsg` field, alongside `clientIp` and
  `type`. The endpoint responds `200 OK` immediately and publishes in the background so that the
  sending rxtx client is not held open.
- **`GET /metrics`** — Prometheus metrics:
    - `rtbeat_batches_received` — total batches received
    - `rtbeat_messages_parsed` — total messages parsed
    - `rtbeat_acks_received` — total publish acknowledgements
    - `rtbeat_current_acks` — most recent ack count

## Quick start

```bash
# build
make build

# run with debug output
./rtbeat -c rtbeat.yml -e -d "*"

# send a batch
curl -s localhost:8081/in -d '{"uuid":"demo","size":1,"messages":[{"seq":"1","payload":{"hello":"world"}}]}'
```

## Docker

```bash
docker run --rm -p 8081:8081 -v "$PWD/rtbeat.yml:/rtbeat.yml" \
  txn2/rtbeat -c /rtbeat.yml -e
```

See [Configuration](configuration.md) for the output and listener settings.
