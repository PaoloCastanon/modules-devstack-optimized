#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "$0")/../.." && pwd)
"$root/scripts/baseline/placement-baseline.sh"
cp "$root"/evidence/sprint2/placement/*.request.txt "$root"/evidence/sprint2/placement/*.status.txt "$root"/evidence/sprint2/placement/*.headers.txt "$root"/evidence/sprint2/placement/*.response.json "$root/tests/fixtures/placement/"
