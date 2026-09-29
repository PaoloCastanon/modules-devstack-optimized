#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
"$root/scripts/baseline/placement-baseline.sh"
"$root/scripts/baseline/glance-baseline.sh"
"$root/scripts/baseline/nova-placement-flow.sh"
echo 'Baseline capturado en evidence/sprint2/{placement,glance,nova}'
