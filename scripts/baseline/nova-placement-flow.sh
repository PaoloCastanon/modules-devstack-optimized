#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "$0")/../.." && pwd)
out="$root/evidence/sprint2/nova"
mkdir -p "$out"
[[ -d /opt/stack/nova/.git ]] || { echo 'Nova no instalado.' >&2; exit 1; }
git -C /opt/stack/nova rev-parse HEAD > "$out/nova-commit.txt"
if [[ -d /opt/stack/logs ]]; then
  rg -i 'allocation_candidates|placement|resource.provider' /opt/stack/logs/n-sch.log* 2>/dev/null |
    sed -E 's/(X-Auth-Token|token|password)[=: ]+[^ ,}]+/\1=[REDACTED]/Ig' > "$out/scheduler-placement.log" || :
fi
[[ -s "$out/scheduler-placement.log" ]] || { echo 'No hay trazas Nova Scheduler/Placement; flujo no validado.' >&2; exit 1; }
