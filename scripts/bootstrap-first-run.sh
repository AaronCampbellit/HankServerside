#!/usr/bin/env bash
set -Eeuo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

env_file=".env.cloud"

log() {
  printf '[bootstrap] %s\n' "$*"
}

fail() {
  printf '[bootstrap] ERROR: %s\n' "$*" >&2
  exit 1
}

truthy() {
  case "${1:-}" in
    1|true|TRUE|yes|YES|y|Y) return 0 ;;
    *) return 1 ;;
  esac
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || fail "$1 is required"
}

prompt_default() {
  local var_name="$1"
  local prompt="$2"
  local default_value="$3"
  local current_value="${!var_name:-}"
  local answer=""

  if [ -n "$current_value" ]; then
    printf -v "$var_name" '%s' "$current_value"
    return
  fi

  if truthy "${HANK_REMOTE_BOOTSTRAP_NONINTERACTIVE:-}" || [ ! -t 0 ]; then
    printf -v "$var_name" '%s' "$default_value"
    return
  fi

  read -r -p "$prompt [$default_value]: " answer
  printf -v "$var_name" '%s' "${answer:-$default_value}"
}

random_hex() {
  local bytes="$1"
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex "$bytes"
    return
  fi
  od -An -N "$bytes" -tx1 /dev/urandom | tr -d ' \n'
  printf '\n'
}

detect_docker_gid() {
  if [ ! -S /var/run/docker.sock ]; then
    printf ''
    return
  fi
  if stat -c '%g' /var/run/docker.sock >/dev/null 2>&1; then
    stat -c '%g' /var/run/docker.sock
    return
  fi
  if stat -f '%g' /var/run/docker.sock >/dev/null 2>&1; then
    stat -f '%g' /var/run/docker.sock
    return
  fi
  printf ''
}

compose() {
  docker compose --env-file "$env_file" "$@"
}

env_value() {
  local key="$1"
  local default_value="$2"
  local value
  value="$(awk -F= -v key="$key" '
    $1 == key {
      sub(/^[^=]*=/, "")
      gsub(/^"/, "")
      gsub(/"$/, "")
      print
      exit
    }
  ' "$env_file")"
  printf '%s' "${value:-$default_value}"
}

wait_for_postgres() {
  local attempt
  for attempt in $(seq 1 60); do
    if compose exec -T postgres pg_isready -U "$POSTGRES_USER" -d "$POSTGRES_DB" >/dev/null 2>&1; then
      return
    fi
    sleep 2
  done
  fail "postgres did not become ready"
}

wait_for_http() {
  local path="$1"
  local label="$2"
  local url="http://127.0.0.1:${HANK_REMOTE_CLOUD_HOST_PORT}${path}"
  local attempt
  for attempt in $(seq 1 60); do
    if curl -fsS "$url" >/dev/null 2>&1; then
      log "$label is ready at $url"
      return
    fi
    sleep 2
  done
  fail "$label did not become ready at $url"
}

write_cloud_env() {
  local db_password db_ops_secret repo_cipher secret_key docker_gid

  prompt_default HANK_REMOTE_BOOTSTRAP_HOST_BIND "Cloud host bind" "127.0.0.1"
  prompt_default HANK_REMOTE_BOOTSTRAP_HOST_PORT "Cloud host port" "18080"
  prompt_default HANK_REMOTE_BOOTSTRAP_POSTGRES_DB "Postgres database name" "hankremote"
  prompt_default HANK_REMOTE_BOOTSTRAP_POSTGRES_USER "Postgres username" "hankremote"

  db_password="${HANK_REMOTE_BOOTSTRAP_POSTGRES_PASSWORD:-$(random_hex 24)}"
  secret_key="${HANK_REMOTE_BOOTSTRAP_SECRET_ENCRYPTION_KEY:-$(random_hex 32)}"
  db_ops_secret="${HANK_REMOTE_BOOTSTRAP_DB_OPS_INTENT_SECRET:-$(random_hex 32)}"
  repo_cipher="${HANK_REMOTE_BOOTSTRAP_DB_OPS_REPO_CIPHER_PASS:-$(random_hex 32)}"
  docker_gid="${HANK_REMOTE_DB_OPS_DOCKER_GID:-$(detect_docker_gid)}"

  umask 077
  local tmp_file
  tmp_file="$(mktemp "${env_file}.tmp.XXXXXX")"
  cat >"$tmp_file" <<EOF
HANK_REMOTE_CLOUD_ADDR=:8080
HANK_REMOTE_CLOUD_HOST_BIND=${HANK_REMOTE_BOOTSTRAP_HOST_BIND}
HANK_REMOTE_CLOUD_HOST_PORT=${HANK_REMOTE_BOOTSTRAP_HOST_PORT}

POSTGRES_DB=${HANK_REMOTE_BOOTSTRAP_POSTGRES_DB}
POSTGRES_USER=${HANK_REMOTE_BOOTSTRAP_POSTGRES_USER}
POSTGRES_PASSWORD=${db_password}
HANK_REMOTE_CLOUD_DATABASE_URL=postgres://${HANK_REMOTE_BOOTSTRAP_POSTGRES_USER}:${db_password}@postgres:5432/${HANK_REMOTE_BOOTSTRAP_POSTGRES_DB}?sslmode=disable

HANK_REMOTE_SESSION_TTL_SECONDS=604800
HANK_REMOTE_REQUEST_TIMEOUT_SECONDS=120
HANK_REMOTE_SECRET_ENCRYPTION_KEY=${secret_key}

HANK_REMOTE_AI_PROVIDER=auto
HANK_REMOTE_OLLAMA_BASE_URL=http://ollama:11434
HANK_REMOTE_OLLAMA_CHAT_MODEL=llama3.1
HANK_REMOTE_OLLAMA_EMBEDDING_MODEL=nomic-embed-text
HANK_REMOTE_PROJECT_DOCS_DIR=/app

HANK_REMOTE_OPENAI_API_KEY=
HANK_REMOTE_OPENAI_CHAT_MODEL=gpt-4o-mini
HANK_REMOTE_OPENAI_EMBEDDING_MODEL=text-embedding-3-small

HANK_REMOTE_CHATGPT_OAUTH_ENABLED=false
HANK_REMOTE_CHATGPT_AUTH_ISSUER=https://auth.openai.com
HANK_REMOTE_CHATGPT_BACKEND_BASE_URL=https://chatgpt.com/backend-api/codex
HANK_REMOTE_CHATGPT_CLIENT_ID=app_EMoamEEZ73f0CkXaXp7hrann
HANK_REMOTE_CHATGPT_CHAT_MODEL=gpt-5.4-mini

HANK_REMOTE_APNS_TEAM_ID=
HANK_REMOTE_APNS_KEY_ID=
HANK_REMOTE_APNS_PRIVATE_KEY=
HANK_REMOTE_APNS_TOPIC=com.dropfile.Hank
HANK_REMOTE_APNS_ENVIRONMENT=sandbox

HANK_REMOTE_DB_OPS_STATE_DIR=/var/lib/hank/db-ops/state
HANK_REMOTE_DB_OPS_LOG_DIR=/var/log/hank/db-ops
HANK_REMOTE_DB_OPS_INTENT_SECRET=${db_ops_secret}
HANK_REMOTE_DB_OPS_REPO_CIPHER_PASS=${repo_cipher}
HANK_REMOTE_DB_OPS_STANZA=hank
HANK_REMOTE_DB_OPS_PGDATA=/var/lib/postgresql/data
HANK_REMOTE_DB_OPS_RESTORE_PGDATA=/var/lib/postgresql/restore
HANK_REMOTE_DB_OPS_RESTORE_DATABASE_URL=postgres://${HANK_REMOTE_BOOTSTRAP_POSTGRES_USER}:${db_password}@postgres-restore:5432/${HANK_REMOTE_BOOTSTRAP_POSTGRES_DB}?sslmode=disable
HANK_REMOTE_DB_OPS_COMPOSE_FILE=/workspace/docker-compose.yml
HANK_REMOTE_DB_OPS_DOCKER_GID=${docker_gid}
EOF

  mv "$tmp_file" "$env_file"
  chmod 600 "$env_file"
  log "created $env_file"
}

if [ -n "${HANK_REMOTE_CLOUD_ENV_FILE:-}" ] && [ "${HANK_REMOTE_CLOUD_ENV_FILE}" != "$env_file" ] && [ "${HANK_REMOTE_CLOUD_ENV_FILE}" != "./$env_file" ]; then
  fail "first-run bootstrap expects repo-root .env.cloud; unset HANK_REMOTE_CLOUD_ENV_FILE for this flow"
fi

require_command docker
require_command curl

docker compose version >/dev/null 2>&1 || fail "Docker Compose v2 is required"

if [ -e "$env_file" ]; then
  if truthy "${HANK_REMOTE_BOOTSTRAP_FORCE:-}"; then
    backup="${env_file}.$(date +%Y%m%d%H%M%S).bak"
    cp "$env_file" "$backup"
    chmod 600 "$backup"
    log "backed up existing $env_file to $backup"
    write_cloud_env
  else
    log "using existing $env_file"
  fi
else
  write_cloud_env
fi

chmod 600 "$env_file"

POSTGRES_USER="$(env_value POSTGRES_USER hankremote)"
POSTGRES_DB="$(env_value POSTGRES_DB hankremote)"
HANK_REMOTE_CLOUD_HOST_PORT="$(env_value HANK_REMOTE_CLOUD_HOST_PORT 18080)"

log "building first-boot images"
if command -v git >/dev/null 2>&1 && git rev-parse --git-dir >/dev/null 2>&1; then
  HANK_REMOTE_BUILD_VERSION="${HANK_REMOTE_BUILD_VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
  HANK_REMOTE_SOURCE_COMMIT="${HANK_REMOTE_SOURCE_COMMIT:-$(git rev-parse HEAD 2>/dev/null || echo unknown)}"
  export HANK_REMOTE_BUILD_VERSION HANK_REMOTE_SOURCE_COMMIT
fi
compose build postgres cloud db-ops

log "starting postgres"
compose up -d postgres
wait_for_postgres

log "running schema migrations"
compose run --rm cloud /usr/local/bin/hank-remote-cloud migrate up
compose run --rm cloud /usr/local/bin/hank-remote-cloud migrate status --strict >/dev/null

log "starting cloud and db-ops"
compose up -d cloud db-ops
wait_for_http "/healthz" "health check"
wait_for_http "/readyz" "readiness check"

log "first boot is complete"
printf '\nNext steps:\n'
printf '  1. Open the public URL or http://127.0.0.1:%s and register the first admin.\n' "$HANK_REMOTE_CLOUD_HOST_PORT"
printf '  2. Create the connector setup file in the dashboard and save it as .env.agent with chmod 600.\n'
printf '  3. Start the connector with: docker compose --env-file .env.cloud --profile agent up -d agent\n'
printf '  4. Run: scripts/doctor.sh\n'
