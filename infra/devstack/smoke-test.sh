#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
out="$root/evidence/sprint2/tests"
mkdir -p "$out"
if [[ -z ${OS_AUTH_URL:-} ]]; then
  [[ -f /opt/stack/devstack/openrc ]] || { echo 'Falta openrc.' >&2; exit 1; }
  # shellcheck disable=SC1091
  source /opt/stack/devstack/openrc admin admin
fi
"$root/infra/devstack/verify-devstack.sh" > "$out/smoke-verify.txt"
provider=$(openstack resource provider list -f value -c uuid | head -1)
[[ -n $provider ]] || { echo 'No hay resource provider.' >&2; exit 1; }
openstack resource provider show "$provider" -f json > "$out/provider.json"
openstack resource provider inventory list "$provider" -f json > "$out/inventory.json"
image=$(openstack image list -f value -c ID | head -1)
if [[ -n $image ]]; then openstack image show "$image" -f json > "$out/image.json"; fi
"$root/scripts/baseline/placement-baseline.sh" --smoke
"$root/scripts/baseline/glance-baseline.sh" --smoke
"$root/scripts/baseline/boot-instance-test.sh"
echo "Smoke tests correctos. Evidencia: $out"
