# Configuration

rtbeat is configured with a libbeat-style YAML file (`rtbeat.yml`). The full set of output,
processor, and logging options is documented in `rtbeat.reference.yml` at the repository root.

## Listener

```yaml
rtbeat:
  # HTTP port the /in and /metrics endpoints listen on.
  port: "8081"
  # Seconds POST /in waits for the output to acknowledge a batch (see Delivery).
  timeout: 5
  # Seconds Stop() waits to drain in-flight events on graceful shutdown.
  shutdown_timeout: 30
```

The defaults (port `8081`, timeout `5`, shutdown_timeout `30`) are applied when the keys are omitted.

## Delivery semantics

rtbeat acknowledges a batch only **after** the configured output confirms delivery:

- `POST /in` publishes the batch with `GuaranteedSend` and waits up to `timeout` seconds for the
  output to ack. On ack it returns **200** ("delivered"); if the ack does not arrive in time it
  returns **504**, so a durable sender such as [rxtx](https://github.com/txn2/rxtx) keeps its copy
  and retries rather than dropping it on a premature 200. (Use a deterministic document id downstream
  so a retried batch dedupes.)
- On graceful shutdown, rtbeat stops accepting new requests, lets in-flight handlers finish, then
  drains outstanding events for up to `shutdown_timeout` seconds before closing the publisher.

For at-least-once delivery across hard crashes as well, configure libbeat's disk queue/spool in the
output/queue settings (see `rtbeat.reference.yml`).

## Output

rtbeat uses the standard libbeat outputs. A minimal Elasticsearch output:

```yaml
output.elasticsearch:
  hosts: ["localhost:9200"]
```

Or ship to Logstash:

```yaml
output.logstash:
  hosts: ["localhost:5044"]
```

Kafka, Redis, file, and console outputs are all supported — see `rtbeat.reference.yml` for the
complete reference, including TLS, authentication, and processor pipelines.

## Event shape

Each rxtx message is published as a Beats event with these fields:

| Field      | Description                                  |
| ---------- | -------------------------------------------- |
| `type`     | The beat name (`rtbeat`).                    |
| `rxtxMsg`  | The original rxtx message (seq, time, uuid, producer, label, key, payload). |
| `clientIp` | The IP address that posted the batch.        |

## Running

```bash
./rtbeat -c rtbeat.yml -e -d "*"
```

- `-c` — path to the config file
- `-e` — log to stderr
- `-d "*"` — enable debug selectors (use sparingly in production)
