#!/usr/bin/env bash
set -euo pipefail
curl -fsS http://localhost:8080/healthz >/dev/null
curl -fsS http://localhost:8090/healthz >/dev/null
curl -fsS http://localhost:8091/healthz >/dev/null
echo "Smoke checks passed"
