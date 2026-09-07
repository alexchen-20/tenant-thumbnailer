#!/usr/bin/env bash
# Onboard-to-thumbnail round trip against a locally running binary.
set -euo pipefail

HOST="${HOST:-http://localhost:8080}"
IMAGE="${1:?usage: ./demo.sh path/to/product.jpg}"

echo "== globex (scale, active): three crops =="
curl -sS -X POST --data-binary "@${IMAGE}" \
  "${HOST}/thumbnails?tenant=globex&asset=hero-01&filename=$(basename "${IMAGE}")"
echo

echo "== acme is suspended by an admin =="
curl -sS "${HOST}/admin?tenant=acme&action=suspend"
echo

echo "== acme now renders nothing (403) =="
curl -sS -o /dev/null -w '%{http_code}\n' -X POST --data-binary "@${IMAGE}" \
  "${HOST}/thumbnails?tenant=acme&asset=hero-01&filename=$(basename "${IMAGE}")"
