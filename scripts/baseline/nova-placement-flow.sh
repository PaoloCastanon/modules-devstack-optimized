#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "$0")/../.." && pwd)
out="$root/evidence/sprint2/nova"
mkdir -p "$out"
[[ -d /opt/stack/nova/.git ]] || { echo 'Nova no instalado.' >&2; exit 1; }
git -C /opt/stack/nova rev-parse HEAD > "$out/nova-commit.txt"
shopt -s nullglob
logs=(/opt/stack/logs/n-sch.log*)
((${#logs[@]})) || { echo 'No hay logs Nova Scheduler.' >&2; exit 1; }
grep -Eih 'allocation_candidates|placement|resource.provider' "${logs[@]}" |
  grep -Eiv 'token|password|cookie|authorization|bearer' > "$out/scheduler-placement.log" || { echo 'No hay trazas sanitizables de Placement.' >&2; exit 1; }
[[ -s "$out/scheduler-placement.log" ]] || { echo 'No hay trazas Nova Scheduler/Placement; flujo no validado.' >&2; exit 1; }
