#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "$0")/../.." && pwd)
"$root/scripts/baseline/glance-baseline.sh"
cp "$root"/evidence/sprint2/glance/*.request.txt "$root"/evidence/sprint2/glance/*.status.txt "$root"/evidence/sprint2/glance/*.headers.txt "$root"/evidence/sprint2/glance/*.response.json "$root/tests/fixtures/glance/"
