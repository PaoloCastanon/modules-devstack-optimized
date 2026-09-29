#!/usr/bin/env bash
set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "$0")/common.sh"
root=$(cd "$(dirname "$0")/../.." && pwd)
out="$root/evidence/sprint2/glance"
base=$(get_endpoint image)
[[ -n $base ]] || { echo 'Endpoint Image ausente.' >&2; exit 1; }
base=${base%/}; base=${base%/v2}
capture versions GET "$base/" "$out"
capture images GET "$base/v2/images" "$out"
image=$(openstack image list -f value -c ID | head -1)
if [[ -n $image ]]; then
  capture image GET "$base/v2/images/$image" "$out"
  capture image-file HEAD "$base/v2/images/$image/file" "$out"
fi
