#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
out="$root/evidence/sprint2/devstack"
mkdir -p "$out"
if [[ -z ${OS_AUTH_URL:-} ]]; then
  [[ -f /opt/stack/devstack/openrc ]] || { echo 'Falta autenticación: source /opt/stack/devstack/openrc admin admin' >&2; exit 1; }
  # shellcheck disable=SC1091
  source /opt/stack/devstack/openrc admin admin
fi
openstack token issue -f value -c expires > "$out/token-expiry.txt"
for spec in 'service list' 'endpoint list' 'compute service list' 'image list' 'flavor list' 'network list' 'resource provider list' 'server list'; do
  name=${spec// /-}
  openstack $spec -f json > "$out/$name.json"
done
python3 - "$out" <<'PY'
import json,sys
from pathlib import Path
p=Path(sys.argv[1]); rows=json.loads((p/'endpoint-list.json').read_text())
present={str(r.get('Service Type',r.get('Service Name',''))).lower() for r in rows}
missing={'image','placement','compute','identity'}-present
if missing: raise SystemExit('Faltan endpoints: '+', '.join(sorted(missing)))
print('Endpoints image, placement, compute, identity presentes')
PY
echo "Verificación correcta. Evidencia: $out"
