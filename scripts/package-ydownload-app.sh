#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="${ROOT}/dist/ydownload.hankapp"
TMP="$(mktemp -d)"
trap 'rm -rf "${TMP}"' EXIT

mkdir -p "${TMP}/bin" "${TMP}/schemas" "${ROOT}/dist"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "${TMP}/bin/hank-app-ydownload" "${ROOT}/cmd/hank-app-ydownload"
cp "${ROOT}/packages/ydownload/app.json" "${TMP}/app.json"
cp "${ROOT}/packages/ydownload/schemas/"*.json "${TMP}/schemas/"
cp "${ROOT}/packages/ydownload/README.md" "${TMP}/README.md" 2>/dev/null || true
python3 - "${TMP}" "${OUT}" <<'PY'
import os
import sys
import zipfile

root, out = sys.argv[1], sys.argv[2]
with zipfile.ZipFile(out, "w", compression=zipfile.ZIP_DEFLATED) as archive:
    for current, _, files in os.walk(root):
        for name in sorted(files):
            path = os.path.join(current, name)
            archive.write(path, os.path.relpath(path, root))
PY
echo "${OUT}"
