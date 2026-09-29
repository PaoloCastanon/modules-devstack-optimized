#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "$0")/../.." && pwd)
out="$root/evidence/sprint2/tests"
mkdir -p "$out"
image=$(openstack image list --status active -f value -c ID | head -1)
flavor=$(openstack flavor list -f value -c ID | head -1)
network=$(openstack network list -f value -c ID | head -1)
[[ -n $image && -n $flavor && -n $network ]] || { echo 'Falta imagen active, flavor o red: boot omitido y smoke no aprobado.' >&2; exit 1; }
name="sprint2-smoke-test-$(date -u +%Y%m%dT%H%M%SZ)"
server=''
cleanup() { if [[ -n $server ]]; then openstack server delete "$server"; fi; }
trap cleanup EXIT
server=$(openstack server create --image "$image" --flavor "$flavor" --network "$network" -f value -c id "$name")
printf '%s\n' "$server" > "$out/boot-server-id.txt"
for attempt in {1..30}; do
  state=$(openstack server show "$server" -f value -c status)
  printf '%s attempt=%s state=%s\n' "$(date -u +%FT%TZ)" "$attempt" "$state" >> "$out/boot-states.txt"
  [[ $state == ACTIVE ]] && { echo 'VM ACTIVE'; exit 0; }
  [[ $state == ERROR ]] && { openstack server show "$server" -f json > "$out/boot-error.json"; exit 1; }
  sleep 10
done
echo 'Timeout esperando ACTIVE.' >&2; exit 1
