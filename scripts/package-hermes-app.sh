#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="${ROOT}/dist/hermes.hankapp"
TMP="$(mktemp -d)"
trap 'rm -rf "${TMP}"' EXIT

mkdir -p "${TMP}/bin" "${TMP}/schemas" "${ROOT}/dist"
go build -o "${TMP}/bin/hank-app-hermes" "${ROOT}/cmd/hank-app-hermes"
cp "${ROOT}/packages/hermes/app.json" "${TMP}/app.json"
cp "${ROOT}/packages/hermes/schemas/"*.json "${TMP}/schemas/"
cp "${ROOT}/packages/hermes/README.md" "${TMP}/README.md" 2>/dev/null || true
(cd "${TMP}" && zip -qr "${OUT}" .)
echo "${OUT}"
