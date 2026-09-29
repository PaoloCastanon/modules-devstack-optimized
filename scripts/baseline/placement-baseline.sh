#!/usr/bin/env bash
set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "$0")/common.sh"
root=$(cd "$(dirname "$0")/../.." && pwd)
out="$root/evidence/sprint2/placement"
base=$(get_endpoint placement)
[[ -n $base ]] || { echo 'Endpoint Placement ausente.' >&2; exit 1; }
base=${base%/}
capture versions GET "$base/" "$out"
capture providers GET "$base/resource_providers" "$out" 1.0
provider=$(openstack resource provider list -f value -c uuid | head -1)
if [[ -n $provider ]]; then
  capture provider GET "$base/resource_providers/$provider" "$out" 1.0
  capture inventories GET "$base/resource_providers/$provider/inventories" "$out" 1.0
  capture traits GET "$base/resource_providers/$provider/traits" "$out" 1.6
fi
capture classes GET "$base/resource_classes" "$out" 1.2
capture candidates GET "$base/allocation_candidates?resources=VCPU%3A1%2CMEMORY_MB%3A64" "$out" 1.10
missing=00000000-0000-4000-8000-000000000000
capture provider-not-found GET "$base/resource_providers/$missing" "$out" 1.0 404
if [[ ${1:-} != --smoke ]]; then
  scratch=$(mktemp -d)
  created=''
  cleanup() { if [[ -n $created ]]; then openstack resource provider delete "$created"; fi; rm -r "$scratch"; }
  trap cleanup EXIT
  created=$(python3 -c 'import uuid; print(uuid.uuid4())')
  printf '{"uuid":"%s","name":"sprint2-baseline-%s"}\n' "$created" "$created" > "$scratch/provider.json"
  capture provider-create POST "$base/resource_providers" "$out" 1.0 201 "$scratch/provider.json"
  capture provider-created GET "$base/resource_providers/$created" "$out" 1.0
  printf '%s\n' '{"resource_provider_generation":0,"inventories":{"VCPU":{"total":1,"reserved":0,"min_unit":1,"max_unit":1,"step_size":1,"allocation_ratio":1.0}}}' > "$scratch/inventory.json"
  capture inventory-put PUT "$base/resource_providers/$created/inventories" "$out" 1.0 200 "$scratch/inventory.json"
  capture inventory-created GET "$base/resource_providers/$created/inventories/VCPU" "$out" 1.0
  capture provider-delete DELETE "$base/resource_providers/$created" "$out" 1.0 204
  created=''
fi
