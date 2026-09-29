#!/usr/bin/env bash
set -euo pipefail
if [[ -z ${OS_AUTH_URL:-} ]]; then
  [[ -f /opt/stack/devstack/openrc ]] || { echo 'Falta OS_AUTH_URL/openrc.' >&2; exit 1; }
  # shellcheck disable=SC1091
  source /opt/stack/devstack/openrc admin admin
fi
get_endpoint() {
  openstack endpoint list --service "$1" --interface public -f value -c URL | head -1
}
get_token() { openstack token issue -f value -c id; }
capture() {
  local label=$1 method=$2 url=$3 out=$4 micro=${5:-}
  local token status
  token=$(get_token)
  mkdir -p "$out"
  printf '%s %s\n' "$method" "$url" > "$out/$label.request.txt"
  printf 'Accept: application/json\n' >> "$out/$label.request.txt"
  if [[ -n $micro ]]; then printf 'OpenStack-API-Version: placement %s\n' "$micro" >> "$out/$label.request.txt"; fi
  local -a args=(-sS -X "$method" -H "X-Auth-Token: $token" -H 'Accept: application/json' -D "$out/$label.headers.tmp" -o "$out/$label.response.json" -w '%{http_code}' "$url")
  if [[ -n $micro ]]; then args+=(-H "OpenStack-API-Version: placement $micro"); fi
  status=$(curl "${args[@]}")
  printf '%s\n' "$status" > "$out/$label.status.txt"
  sed -E '/^[Xx]-[Aa]uth-[Tt]oken:/d; /^[Xx]-[Ss]ubject-[Tt]oken:/d; /^[Ss]et-[Cc]ookie:/d' "$out/$label.headers.tmp" > "$out/$label.headers.txt"
  rm "$out/$label.headers.tmp"
  [[ $status =~ ^2[0-9][0-9]$ ]] || { echo "$label: HTTP $status" >&2; return 1; }
}
