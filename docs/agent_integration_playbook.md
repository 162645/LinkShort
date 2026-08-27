# NexusLink Agent Integration Playbook

This playbook defines the minimum observability contract expected by the oncall agent.

## Health endpoints

- REST API: `GET /health` on `:8080`
- Prometheus metrics: `GET /metrics` on `:8080`

## Core PromQL used by agent

- Total request rate:
  - `sum(rate(http_requests_total[5m]))`
- 5xx error ratio:
  - `sum(rate(http_requests_total{status=~"5.."}[5m])) / sum(rate(http_requests_total[5m]))`
- Redirect P99 latency:
  - `histogram_quantile(0.99, sum(rate(redirect_request_duration_seconds_bucket[5m])) by (le))`
- L1 cache hit ratio:
  - `sum(rate(cache_hits_total{layer="l1",result="hit"}[5m])) / sum(rate(cache_hits_total{layer="l1"}[5m]))`

## Oncall drill checklist

1. Start stack (`docker compose up -d`).
2. Confirm `/health` and `/metrics`.
3. Generate traffic with `scripts/ops/generate_hotspot_traffic.sh`.
4. Verify Prometheus receives samples.
5. Trigger agent tool calls:
   - `query_prometheus_alerts`
   - `query_nexuslink_health`

