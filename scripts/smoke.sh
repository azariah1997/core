#!/usr/bin/env bash
set -euo pipefail
curl -fsS http://localhost:8080/healthz >/dev/null
curl -fsS http://localhost:8090/healthz >/dev/null
curl -fsS http://localhost:8091/healthz >/dev/null

# Phase 28: a real Prometheus exposition endpoint, not just a 200 -
# confirms the metrics middleware is actually wired into each service's
# router, not just that the process is alive.
for svc in "core-api:8080" "realtime-gateway:8090" "worker:8091"; do
  name="${svc%%:*}"; port="${svc##*:}"
  body="$(curl -fsS "http://localhost:${port}/metrics")"
  if ! grep -q '^http_requests_total' <<<"$body"; then
    echo "smoke: ${name}'s /metrics is missing http_requests_total" >&2
    exit 1
  fi
done

echo "Smoke checks passed"
