#!/usr/bin/env bash
set -euo pipefail

base_url="${HANK_REMOTE_METRICS_BASE_URL:-${HANK_REMOTE_LOADTEST_BASE_URL:-}}"
token="${HANK_REMOTE_METRICS_SESSION_TOKEN:-${HANK_REMOTE_LOADTEST_SESSION_TOKEN:-}}"
if [[ -z "$base_url" || -z "$token" ]]; then
  echo "set HANK_REMOTE_METRICS_BASE_URL and HANK_REMOTE_METRICS_SESSION_TOKEN" >&2
  exit 2
fi

metrics="$(curl -fsS -H "Authorization: Bearer $token" "${base_url%/}/metrics")"
required=(
  "hank_remote_http_requests_total"
  "hank_remote_http_latency_seconds_sum"
  "hank_remote_relay_latency_seconds_sum"
  "hank_remote_db_backup_last_success_unixtime"
  "hank_remote_db_restore_test_last_success_unixtime"
  "hank_remote_attachment_storage_bytes"
  "hank_remote_file_operation_jobs"
  "hank_remote_assistant_provider_requests_total"
  "hank_remote_go_alloc_bytes"
  "hank_remote_db_open_connections"
  "hank_remote_db_ping_success"
  "hank_remote_cloud_runtime_heartbeat_age_seconds"
  "hank_remote_cloud_runtime_up"
)

missing=0
for name in "${required[@]}"; do
  if ! grep -q "^$name" <<<"$metrics"; then
    echo "missing metric: $name" >&2
    missing=1
  fi
done

if [[ "$missing" -ne 0 ]]; then
  exit 1
fi

echo "metrics assertions passed"
