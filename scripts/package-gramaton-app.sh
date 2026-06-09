#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="${ROOT}/dist/gramaton.hankapp"
TMP="$(mktemp -d)"
trap 'rm -rf "${TMP}"' EXIT

mkdir -p "${TMP}/bin" "${TMP}/schemas" "${ROOT}/dist"
go build -o "${TMP}/bin/hank-app-gramaton" "${ROOT}/cmd/hank-app-gramaton"
cp "${ROOT}/packages/gramaton/app.json" "${TMP}/app.json"
cp "${ROOT}/packages/gramaton/schemas/"*.json "${TMP}/schemas/"
cp "${ROOT}/packages/gramaton/README.md" "${TMP}/README.md" 2>/dev/null || true
(cd "${TMP}" && zip -qr "${OUT}" .)
echo "${OUT}"
