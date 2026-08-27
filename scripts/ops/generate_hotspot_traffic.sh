#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
SHORT_CODE="${SHORT_CODE:-demo123}"
DURATION_SECONDS="${DURATION_SECONDS:-30}"
CONCURRENCY="${CONCURRENCY:-40}"

echo "[nexuslink-ops] base_url=${BASE_URL}"
echo "[nexuslink-ops] short_code=${SHORT_CODE}"
echo "[nexuslink-ops] duration_seconds=${DURATION_SECONDS}"
echo "[nexuslink-ops] concurrency=${CONCURRENCY}"

if command -v wrk >/dev/null 2>&1; then
  echo "[nexuslink-ops] using wrk to generate hotspot traffic"
  wrk -t4 -c"${CONCURRENCY}" -d"${DURATION_SECONDS}s" "${BASE_URL}/${SHORT_CODE}"
  exit 0
fi

echo "[nexuslink-ops] wrk not found, fallback to curl loop"
end_ts=$(( $(date +%s) + DURATION_SECONDS ))
while [ "$(date +%s)" -lt "${end_ts}" ]; do
  for _ in $(seq 1 "${CONCURRENCY}"); do
    curl -s -o /dev/null "${BASE_URL}/${SHORT_CODE}" &
  done
  wait
done

echo "[nexuslink-ops] done"

