#!/usr/bin/env bash
set -euo pipefail
[[ -d /opt/stack/devstack/.git ]] || { echo 'DevStack no instalado.' >&2; exit 1; }
cd /opt/stack/devstack
./unstack.sh
