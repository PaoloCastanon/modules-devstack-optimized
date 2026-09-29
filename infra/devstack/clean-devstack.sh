#!/usr/bin/env bash
set -euo pipefail
[[ ${DEDICATED_LAB:-NO} == YES ]] || { echo 'Exige DEDICATED_LAB=YES en VM dedicada.' >&2; exit 1; }
[[ -d /opt/stack/devstack/.git ]] || { echo 'DevStack no instalado.' >&2; exit 1; }
cd /opt/stack/devstack
./clean.sh
