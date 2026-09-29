#!/usr/bin/env bash
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck disable=SC1091
source "$here/versions.lock"
dir=${DEVSTACK_DIR:-/opt/stack/devstack}
for service in placement glance nova; do
  path="/opt/stack/$service"
  [[ -d $path/.git ]] || { echo "Falta checkout: $path" >&2; exit 1; }
done
source /etc/os-release
db_version=$(mariadb --version 2>/dev/null || mysql --version 2>/dev/null || echo UNAVAILABLE)
osc_version=$(openstack --version 2>&1 || echo UNAVAILABLE)
cat > "$here/versions.lock" <<EOF
# Actualizado desde instalación local; no contiene secretos.
DEVSTACK_BRANCH=$DEVSTACK_BRANCH
DEVSTACK_SHA=$(git -C "$dir" rev-parse HEAD)
PLACEMENT_BRANCH=$PLACEMENT_BRANCH
PLACEMENT_SHA=$(git -C /opt/stack/placement rev-parse HEAD)
GLANCE_BRANCH=$GLANCE_BRANCH
GLANCE_SHA=$(git -C /opt/stack/glance rev-parse HEAD)
NOVA_BRANCH=$NOVA_BRANCH
NOVA_SHA=$(git -C /opt/stack/nova rev-parse HEAD)
OS_VERSION='${PRETTY_NAME}'
PYTHON_VERSION='$(python3 --version)'
DATABASE_VERSION='${db_version}'
OPENSTACKCLIENT_VERSION='${osc_version}'
EOF
echo "Versiones registradas: $here/versions.lock"
