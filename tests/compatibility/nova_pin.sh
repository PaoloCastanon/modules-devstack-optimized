#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
# shellcheck disable=SC1091
source "$root/infra/devstack/versions.lock"

expected="247ea5d774305b688997f8df81c011b249eb793c"
[[ "$NOVA_SHA" == "$expected" ]] || { echo "El SHA fijado de Nova cambió." >&2; exit 1; }

template="$root/infra/devstack/local.conf.template"
[[ $(grep -cx 'NOVA_BRANCH=@NOVA_SHA@' "$template") -eq 1 ]] || {
  echo "El template debe configurar Nova mediante NOVA_SHA." >&2
  exit 1
}

rendered=$(sed "s/@NOVA_SHA@/$NOVA_SHA/g" "$template")
grep -qx "NOVA_BRANCH=$NOVA_SHA" <<< "$rendered" || {
  echo "El pin de Nova no se aplica a local.conf." >&2
  exit 1
}
echo "Nova fijado a $NOVA_SHA; plantilla verificada."
