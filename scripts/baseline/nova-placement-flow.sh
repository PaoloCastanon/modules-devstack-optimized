#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "$0")/../.." && pwd)
out="$root/evidence/sprint2/nova"
mkdir -p "$out"
[[ -d /opt/stack/nova/.git ]] || { echo 'Nova no instalado.' >&2; exit 1; }
git -C /opt/stack/nova rev-parse HEAD > "$out/nova-commit.txt"
shopt -s nullglob
logs=(/opt/stack/logs/n-sch.log*)
if ((${#logs[@]})); then
  source_log() { cat "${logs[@]}"; }
else
  source_log() { sudo -n journalctl -u devstack@n-sch.service --no-pager -o cat; }
fi
source_log | grep -Ei 'allocation_candidates|placement|resource.provider' |
  grep -Eiv 'token|password|cookie|authorization|bearer' > "$out/scheduler-placement.log" || { echo 'No hay trazas sanitizables de Placement.' >&2; exit 1; }
[[ -s "$out/scheduler-placement.log" ]] || { echo 'No hay trazas Nova Scheduler/Placement; flujo no validado.' >&2; exit 1; }
grep -Eiq 'claim resources in the placement API|allocation_candidates' "$out/scheduler-placement.log" || { echo 'No hay evidencia de consulta/asignación de Placement.' >&2; exit 1; }
sudo -n journalctl -u devstack@placement-api.service --no-pager -o cat |
  grep -Ei 'INFO placement.requestlog' |
  grep -E 'service nova' |
  grep -E 'allocation_candidates|/allocations/' |
  grep -Eiv 'token|password|cookie|authorization|bearer' > "$out/placement-requests.log" || { echo 'No hay solicitudes Nova→Placement en journald.' >&2; exit 1; }
