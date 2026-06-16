# Configuration

rtbeat is configured with a libbeat-style YAML file (`rtbeat.yml`). The full set of output,
processor, and logging options is documented in `rtbeat.reference.yml` at the repository root.

## Listener

```yaml
rtbeat:
  # HTTP port the /in and /metrics endpoints listen on.
  port: "8081"
  # Read timeout, in seconds.
  timeout: 5
```

The defaults (port `8081`, timeout `5`) are applied when the keys are omitted.

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
