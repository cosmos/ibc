<!--
  ~ SPDX-License-Identifier: Apache-2.0
-->

# Local metrics stack

Single-container [grafana/otel-lgtm](https://github.com/grafana/docker-otel-lgtm) for local
IBC metrics testing. Uses `network_mode: host` so the embedded collector can reach the binary on the host.

## Usage

1. Enable metrics in IBC config (`type: simple`):

```yaml
observability:
  metrics: true
  type: simple
  listenAddr: 0.0.0.0:9090
```

2. Start the stack:

```bash
make -C cli/scripts/otel up
# or: make otel-up   (from cli/)
```

3. Run IBC, then open Grafana at http://localhost:3001 (admin / admin).

## Ports

| What                   | Port              |
| ---------------------- | ----------------- |
| IBC metrics (`simple`) | `:9090`           |
| IBC API                | `:3000`           |
| Grafana                | `:3001`           |
| Prometheus             | `:9091`           |
| OTLP (future)          | `:4317` / `:4318` |

Prometheus/Grafana ports are shifted to avoid clashing with IBC defaults.

## TODO: `observability.type: otel`

Today only `simple` works — the CLI exposes a Prometheus `/metrics` endpoint and this
stack scrapes it via `otelcol-config.yaml`.