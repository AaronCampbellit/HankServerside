#!/usr/bin/env bash
set -Eeuo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

env_file=".env.cloud"
failures=0
warnings=0

pass() {
  printf '[doctor] ok: %s\n' "$*"
}

warn() {
  warnings=$((warnings + 1))
  printf '[doctor] warn: %s\n' "$*" >&2
}

fail() {
  failures=$((failures + 1))
  printf '[doctor] fail: %s\n' "$*" >&2
}

command_exists() {
  command -v "$1" >/dev/null 2>&1
}

file_mode() {
  local path="$1"
  if stat -c '%a' "$path" >/dev/null 2>&1; then
    stat -c '%a' "$path"
    return
  fi
  stat -f '%Lp' "$path"
}

check_secret_mode() {
  local path="$1"
  local mode
  mode="$(file_mode "$path")"
  if (( (8#$mode & 077) != 0 )); then
    fail "$path mode is $mode; run chmod 600 $path"
  else
    pass "$path mode is $mode"
  fi
}

env_value() {
  local key="$1"
  awk -F= -v key="$key" '
    $1 == key {
      sub(/^[^=]*=/, "")
      gsub(/^"/, "")
      gsub(/"$/, "")
      print
      exit
    }
  ' "$env_file"
}

compose() {
  docker compose --env-file "$env_file" "$@"
}

if [ -n "${HANK_REMOTE_CLOUD_ENV_FILE:-}" ] && [ "${HANK_REMOTE_CLOUD_ENV_FILE}" != "$env_file" ] && [ "${HANK_REMOTE_CLOUD_ENV_FILE}" != "./$env_file" ]; then
  fail "doctor expects repo-root .env.cloud; unset HANK_REMOTE_CLOUD_ENV_FILE for production Compose checks"
fi

if command_exists docker; then
  pass "docker command is available"
else
  fail "docker command is missing"
fi

if command_exists docker && docker compose version >/dev/null 2>&1; then
  pass "docker compose v2 is available"
else
  fail "docker compose v2 is missing"
fi

if command_exists curl; then
  pass "curl command is available"
else
  fail "curl command is missing"
fi

if [ -f "$env_file" ]; then
  pass "$env_file exists"
  check_secret_mode "$env_file"
else
  fail "$env_file is missing; run scripts/bootstrap-first-run.sh first"
fi

if [ -f ".env.agent" ]; then
  pass ".env.agent exists"
  check_secret_mode ".env.agent"
else
  warn ".env.agent is not present yet; the connector has not been started"
fi

if [ "$failures" -eq 0 ]; then
  if compose config --quiet >/dev/null 2>&1; then
    pass "docker compose config is valid"
  else
    fail "docker compose config is invalid"
  fi

  printf '\n[doctor] compose services:\n'
  compose ps || fail "docker compose ps failed"

  port="$(env_value HANK_REMOTE_CLOUD_HOST_PORT)"
  port="${port:-18080}"
  if curl -fsS "http://127.0.0.1:${port}/healthz" >/dev/null 2>&1; then
    pass "cloud /healthz responds on localhost:${port}"
  else
    fail "cloud /healthz did not respond on localhost:${port}"
  fi

  if curl -fsS "http://127.0.0.1:${port}/readyz" >/dev/null 2>&1; then
    pass "cloud /readyz responds on localhost:${port}"
  else
    fail "cloud /readyz did not respond on localhost:${port}"
  fi

  if compose run --rm --entrypoint /usr/local/bin/hank-remote-cloud cloud migrate status --strict >/tmp/hank-remote-migration-status.$$ 2>/tmp/hank-remote-migration-status-err.$$; then
    pass "migration status is strict-clean"
  else
    fail "migration status check failed"
    sed -n '1,80p' /tmp/hank-remote-migration-status-err.$$ >&2 || true
  fi
  rm -f /tmp/hank-remote-migration-status.$$ /tmp/hank-remote-migration-status-err.$$

  running_services="$(compose ps --services --status running 2>/dev/null || true)"
  if printf '%s\n' "$running_services" | grep -qx "db-ops"; then
    if compose exec -T db-ops docker version >/dev/null 2>&1; then
      pass "db-ops can reach the Docker socket"
    else
      fail "db-ops cannot reach the Docker socket; check HANK_REMOTE_DB_OPS_DOCKER_GID"
    fi
  else
    fail "db-ops is not running"
  fi

  if [ -f ".env.agent" ]; then
    agent_running_services="$(docker compose --env-file "$env_file" --profile agent ps --services --status running 2>/dev/null || true)"
    if printf '%s\n' "$agent_running_services" | grep -qx "agent"; then
      pass "agent is running"
    else
      fail ".env.agent exists but the agent service is not running"
    fi
  fi
fi

printf '\n[doctor] complete: %d failure(s), %d warning(s)\n' "$failures" "$warnings"
if [ "$failures" -ne 0 ]; then
  exit 1
fi
