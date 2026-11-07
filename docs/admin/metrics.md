# Metrics & Grafana

- Prometheus metrics **`GET /metrics`** 
- Health: **`GET /api/health`**

## Example Prometheus scrape config

```yaml
scrape_configs:
  - job_name: sudostream
    static_configs:
      - targets: ["sudostream:8080"]
```

## Grafana dashboard

Import the JSON shipped with the docs:

**[`assets/grafana-dashboard.json`](../assets/grafana-dashboard.json)**{: download="grafana-dashboard.json" }

1. Grafana → **Dashboards → New → Import**
2. Upload `grafana-dashboard.json` (or paste)
3. Select your Prometheus datasource (`DS_PROMETHEUS`)

## Useful series

Curated app metrics (not every Go default). Names are as exported — no `sudostream_` prefix.

### At a glance (gauges)

| Metric | Why watch |
|--------|-----------|
| `transcode_jobs_active` | HLS timeline probe/publish jobs in flight |
| `transcode_segment_sessions_active` | Live ffmpeg segment encode sessions — encode load |
| `playback_streams_active` | Progressive streams currently served |
| `hls_cache_bytes` / `hls_cache_entries` | On-disk HLS cache footprint |
| `http_requests_in_flight` | Concurrent HTTP work |
| `maintenance_running` | Scheduled job currently running (by action) |
| `auth_sessions_active` | Stored refresh sessions |
| `library_media_files` | Indexed files per library slug |
| `trash_items` | Recycle-bin depth |
| `db_sql_open_conns` | Postgres pool pressure |

### Playback & HLS

| Metric | Why watch |
|--------|-----------|
| `playback_decisions_total` | Share of Direct Play vs remux vs transcode (by client) |
| `playback_stream_bytes_total` | Progressive bandwidth |
| `video_player_opens_total` | Player open rate by library type |
| `hls_cache_lookups_total` | Cache hit / miss |
| `hls_segment_requests_total` | How segments were served |
| `hls_throttle_events_total` | Encoder pause/resume (ahead-of-playhead throttle) |
| `transcode_timeline_jobs_total` / `_duration_seconds` | Timeline job outcomes & latency |
| `transcode_segment_jobs_total` / `_duration_seconds` | Segment encode outcomes & latency |
| `ffmpeg_errors_total` | Failures by stage |

### Library ops & auth

| Metric | Why watch |
|--------|-----------|
| `metadata_probe_total` / `_duration_seconds` | Scan ffprobe rate & latency |
| `metadata_index_files_total` | Files indexed per library |
| `video_poster_generations_total` | Thumb warm success/fail |
| `maintenance_runs_total` / `_duration_seconds` | Job outcomes & wall time |
| `provider_task_items_total` | Enrichment item results |
| `provider_http_requests_total` | Outbound provider HTTP by status class |
| `auth_login_attempts_total` | Login success / failure mix |
| `dlna_soap_requests_total` / `dlna_stream_requests_total` | Living-room control & stream health |
| `http_requests_total` / `http_request_duration_seconds` | API traffic & latency |
