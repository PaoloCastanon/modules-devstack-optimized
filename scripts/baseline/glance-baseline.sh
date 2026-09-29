#!/usr/bin/env bash
set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "$0")/common.sh"
root=$(cd "$(dirname "$0")/../.." && pwd)
out="$root/evidence/sprint2/glance"
base=$(get_endpoint image)
[[ -n $base ]] || { echo 'Endpoint Image ausente.' >&2; exit 1; }
base=${base%/}; base=${base%/v2}
capture versions GET "$base/" "$out" '' '2|3'
capture images GET "$base/v2/images" "$out"
image=$(openstack image list -f value -c ID | head -1)
if [[ -n $image ]]; then
  capture image GET "$base/v2/images/$image" "$out"
  # Download is deliberately separate: do not save arbitrary large binaries in Git.
fi
missing=00000000-0000-4000-8000-000000000000
capture image-not-found GET "$base/v2/images/$missing" "$out" '' 404
if [[ ${1:-} != --smoke ]]; then
  scratch=$(mktemp -d)
  created=''
  cleanup() { if [[ -n $created ]]; then openstack image delete "$created"; fi; rm -r "$scratch"; }
  trap cleanup EXIT
  printf '{"name":"sprint2-baseline-%s","disk_format":"raw","container_format":"bare","visibility":"private"}\n' "$(date -u +%Y%m%dT%H%M%SZ)" > "$scratch/image.json"
  capture image-create POST "$base/v2/images" "$out" '' 201 "$scratch/image.json"
  created=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["id"])' "$out/image-create.response.json")
  printf 'sprint2 raw fixture\n' > "$scratch/image.raw"
  capture image-upload PUT "$base/v2/images/$created/file" "$out" '' 204 "$scratch/image.raw" application/octet-stream
  capture image-active GET "$base/v2/images/$created" "$out"
  printf '%s\n' '[{"op":"add","path":"/description","value":"Sprint 2 baseline fixture"}]' > "$scratch/image-patch.json"
  capture image-patch PATCH "$base/v2/images/$created" "$out" '' 200 "$scratch/image-patch.json" application/openstack-images-v2.1-json-patch
  token=$(get_token)
  printf 'GET %s/v2/images/%s/file\nAccept: application/octet-stream\n' "$base" "$created" > "$out/image-download.request.txt"
  status=$(curl -sS -H "X-Auth-Token: $token" -H 'Accept: application/octet-stream' -D "$scratch/download.headers" -o "$scratch/download.raw" -w '%{http_code}' "$base/v2/images/$created/file")
  printf '%s\n' "$status" > "$out/image-download.status.txt"
  sed -E '/^[Xx]-[Aa]uth-[Tt]oken:/d; /^[Xx]-[Ss]ubject-[Tt]oken:/d; /^[Ss]et-[Cc]ookie:/d' "$scratch/download.headers" > "$out/image-download.headers.txt"
  [[ $status == 200 ]] || { echo "Download HTTP $status" >&2; exit 1; }
  sha256sum "$scratch/image.raw" "$scratch/download.raw" | awk '{print $1}' > "$out/image-download.sha256.txt"
  cmp "$scratch/image.raw" "$scratch/download.raw" || { echo 'Download difiere del upload.' >&2; exit 1; }
  capture image-delete DELETE "$base/v2/images/$created" "$out" '' 204
  created=''
fi
