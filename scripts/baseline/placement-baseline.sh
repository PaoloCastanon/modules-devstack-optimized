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
