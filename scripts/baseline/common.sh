#!/usr/bin/env bash
set -euo pipefail
if [[ -z ${OS_AUTH_URL:-} ]]; then
  [[ -f /opt/stack/devstack/openrc ]] || { echo 'Falta OS_AUTH_URL/openrc.' >&2; exit 1; }
  # shellcheck disable=SC1091
  set +u
  source /opt/stack/devstack/openrc admin admin
  set -u
fi
get_endpoint() {
  openstack endpoint list -f json | python3 -c '
import json,sys
rows=json.load(sys.stdin)
for row in rows:
    if str(row.get("Service Type", "")).lower()==sys.argv[1] and str(row.get("Interface", "")).lower()=="public":
        print(row["URL"])
        break
' "$1"
}
get_token() { openstack token issue -f value -c id; }
capture() {
  local label=$1 method=$2 url=$3 out=$4 micro=${5:-} expected=${6:-2} body=${7:-} media=${8:-application/json}
  local token status
  token=$(get_token)
  mkdir -p "$out"
  printf '%s %s\n' "$method" "$url" > "$out/$label.request.txt"
  printf 'Accept: application/json\n' >> "$out/$label.request.txt"
  if [[ -n $micro ]]; then printf 'OpenStack-API-Version: placement %s\n' "$micro" >> "$out/$label.request.txt"; fi
  if [[ -n $body ]]; then printf 'Content-Type: %s\nBody file: %s\n' "$media" "$(basename "$body")" >> "$out/$label.request.txt"; cp "$body" "$out/$label.request.body"; fi
  local -a args=(-sS -X "$method" -H "X-Auth-Token: $token" -H 'Accept: application/json' -D "$out/$label.headers.tmp" -o "$out/$label.response.json" -w '%{http_code}' "$url")
  if [[ -n $micro ]]; then args+=(-H "OpenStack-API-Version: placement $micro"); fi
  if [[ -n $body ]]; then args+=(-H "Content-Type: $media" --data-binary "@$body"); fi
  status=$(curl "${args[@]}")
  printf '%s\n' "$status" > "$out/$label.status.txt"
  sed -E '/^[Xx]-[Aa]uth-[Tt]oken:/d; /^[Xx]-[Ss]ubject-[Tt]oken:/d; /^[Ss]et-[Cc]ookie:/d' "$out/$label.headers.tmp" > "$out/$label.headers.txt"
  rm "$out/$label.headers.tmp"
  if [[ $expected == '2|3' ]]; then
    [[ $status == 2* || $status == 3* ]] || { echo "$label: HTTP $status; expected 2xx/3xx" >&2; return 1; }
  else
    [[ $status == "$expected"* ]] || { echo "$label: HTTP $status; expected $expected*" >&2; return 1; }
  fi
}
