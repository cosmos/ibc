<!--
  ~ SPDX-License-Identifier: Apache-2.0
-->

# Local metrics stack

Single-container [grafana/otel-lgtm](https://github.com/grafana/docker-otel-lgtm) for local
IBC metrics testing. The collector supports both `simple` Prometheus scraping and
OTEL push mode. It uses host networking so it can reach the IBC process.

## Usage

1. Start the stack:

```bash
make -C cli/scripts/otel up
```

2. Select one observability mode in the IBC config.

### Simple mode

IBC exposes `/metrics`; the collector scrapes it:

```yaml
observability:
  metrics: true
  type: simple
  simpleMetricsListenAddr: 127.0.0.1:9090 # so localhost:9090/metrics dumps prometheus metrics
```

### OTEL mode

IBC pushes metrics to the collector using
[`ibc-otel.yaml`](./ibc-otel.yaml):

```yaml
observability:
  metrics: true
  type: otel
  otelFile: scripts/otel/ibc-otel.yaml
```

Relative `otelFile` paths resolve from the process working directory. Alternatively,
set an absolute path with `OTEL_CONFIG_FILE`; the environment variable overrides
`otelFile`:

```bash
OTEL_CONFIG_FILE="$(pwd)/scripts/otel/ibc-otel.yaml" ibc relayer run
```

3. Run IBC, then open Grafana at http://localhost:3001 (`admin` / `admin`).
The stack does not need to be restarted when switching modes.

## Dashboards

Grafana provisions every JSON file in [`dashboards/`](./dashboards) on startup, including
the "IBC Local Metrics" load-test dashboard. The stack keeps no state, so UI edits are lost
on `down`: to keep them, use Export → Export as JSON (Grafana v2 resource format), save it
over [`dashboards/loadtest.json`](./dashboards/loadtest.json), and commit.

## Ports

| Label                                         | Port              | Note                  |
| --------------------------------------------- | ----------------- | --------------------- |
| IBC metrics (for `simple` mode)               | `:9090/metrics`   | `ibc` process metrics |
| IBC API                                       | `:3000`           | `ibc` ConnectRPC API  |
| Grafana                                       | `:3001`           | Dashboards            |
| Prometheus                                    | `:9091`           | Metrics storage       |
| OTLP (OpenTelemetry Protocol for `otel` mode) | `:4317` / `:4318` | Collector ingestion   |
