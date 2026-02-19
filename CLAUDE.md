# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**Goppstats** is a Go daemon that collects partitioned performance (PP) statistics from Dell EMC PowerScale (Isilon) OneFS clusters via the PAPI REST API and routes them to pluggable metric backends.

## Build & Run Commands

```bash
# Build
go build                          # produces goppstats (or goppstats.exe on Windows)
make build                        # same, via Makefile

# Test
go test -v ./...                  # run all tests

# Run
./goppstats                                          # uses ./goppstats.toml
./goppstats -config-file /path/to/config.toml
./goppstats -version
./goppstats -loglevel DEBUG       # levels: CRITICAL|ERROR|WARNING|NOTICE|INFO|DEBUG
```

## Architecture

### Data Flow

```
OneFS Clusters (PAPI REST API)
    └─> isilon_api.go  (per-cluster goroutine, 30s poll interval)
          └─> statssink.go  (DBWriter interface)
                ├── influxdb.go      (InfluxDB v1)
                ├── influxdbv2.go    (InfluxDB v2)
                ├── prometheus.go    (Prometheus pull model, per-cluster HTTP server)
                └── discard.go      (no-op, for testing)
```

One goroutine is spawned per enabled cluster. The poll interval is fixed at 30 seconds (`PPSampleRate`) — this matches the OneFS stats update cycle.

### Key Files

| File | Role |
|------|------|
| `main.go` | Entry point, cluster loop orchestration, backend factory, logging setup |
| `statssink.go` | `DBWriter` interface — contract all backends must implement |
| `config.go` | TOML config parsing, version compatibility checks, `$env:VAR` secret interpolation |
| `isilon_api.go` | OneFS PAPI client, session/basic-auth, PP stats querying |
| `backend.go` | Shared data types (`PPStatResult`, `DsInfo`), field/tag conversion helpers |
| `prometheus.go` | Prometheus backend: per-cluster HTTP `/metrics` server, optional SD endpoint |
| `extnet.go` | External IP detection for Prometheus service discovery |
| `control_unix.go` / `control_windows.go` | Platform-specific socket options (build-tag separated) |

### DBWriter Interface

All backends implement:
```go
type DBWriter interface {
    Init(cluster *Cluster, config *tomlConfig, ci int) error
    UpdateDatasets(di *DsInfo)
    WritePPStats(ds DsInfoEntry, stats []PPStatResult) error
}
```

The backend is selected by `stats_processor` in the config (`"influxdb"`, `"influxdbv2"`, `"prometheus"`, `"discard"`).

### Prometheus Backend Notes

- Runs a separate HTTP listener per cluster (each cluster gets its own `prometheus_port`)
- Uses a pull model — goppstats hosts `/metrics`; Prometheus scrapes it
- Optional HTTP service discovery endpoint (`[prom_http_sd]`) for dynamic cluster registration
- Metrics are named `isilon_ppstat_<metric>_<field>{<labels>}`

### Configuration

Config is TOML. The version field in `[global]` is checked for compatibility (supported: 0.23–0.28). Secrets can reference environment variables via `$env:VARNAME` syntax in any string field. See `example_goppstats.toml` for all available options.

### Platform Compatibility

Windows and Unix have separate socket option files (`control_windows.go`, `control_unix.go`) selected via Go build constraints. Windows supports only `SO_REUSEADDR`; Unix adds `SO_REUSEPORT`.

## Dependencies

- `github.com/BurntSushi/toml` — config parsing
- `github.com/op/go-logging` — logging
- `github.com/prometheus/client_golang` — Prometheus metrics
- `github.com/influxdata/influxdb1-client/v2` — InfluxDB v1
- `github.com/influxdata/influxdb-client-go/v2` — InfluxDB v2
