# Walt.id metrics — Phase 6.1

## Endpoint discovery

Walt.id services (issuer-api, verifier-api, wallet-api) are Ktor apps with
Micrometer. The Prometheus endpoint is **expected to be `/metrics` on the
same port as the API** (7001/7002/7003), but **this must be verified
against a running container** before flipping ServiceMonitor on for prod.

To verify locally once the compose stack is up:

```sh
curl -s http://localhost:7002/metrics | head     # issuer-api
curl -s http://localhost:7003/metrics | head     # verifier-api
curl -s http://localhost:7001/metrics | head     # wallet-api
```

If `/metrics` returns 404, walt.id ships micrometer registered at a
different path (try `/actuator/prometheus`, `/q/metrics`). Update each
chart's `values.yaml` `*.serviceMonitor.path` accordingly.

## Per-chart toggle

Each chart (`walt-issuer`, `walt-verifier`, `walt-wallet`, `verifiably-go`)
has a `serviceMonitor:` block with `enabled: false` by default — flip to
`true` once the endpoint is verified:

```yaml
issuer:
  serviceMonitor:
    enabled: true
    path: /metrics
    interval: 30s
```

The umbrella chart can flip them all at once via `values-prod.yaml`.

## Grafana dashboards

The `kube-prometheus-stack` Grafana sidecar auto-loads any ConfigMap
labeled `grafana_dashboard: "1"` from any namespace. Each chart should
ship a `dashboards/` directory with dashboard JSONs and a small template
that wraps them in a labeled ConfigMap.

Initial dashboards to author:
- **walt-stack-overview**: requests/sec, p50/p95/p99 latency, error rate by
  service. Source: `http_server_requests_seconds` (Micrometer default).
- **walt-wallet-detail**: DB pool stats, active sessions, key-cache hits.
- **verifiably-go**: Go runtime metrics from `expvar` or
  `prometheus/client_golang` (not yet wired — Phase 6.1 follow-up).

## ServiceMonitor selector

`kube-prometheus-stack` is configured (in `terraform/platform/observability.tf`)
with `serviceMonitorSelectorNilUsesHelmValues: false` so it discovers
`ServiceMonitor` resources in any namespace as long as the resource has
the `release: kube-prometheus-stack` label — which our chart templates set.
