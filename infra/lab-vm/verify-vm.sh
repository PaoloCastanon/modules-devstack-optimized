#!/usr/bin/env bash
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
root=$(cd "$here/../.." && pwd)
key=${LAB_SSH_KEY:-$HOME/.ssh/id_ed25519}
ip=$("$here/status.sh" | sed -n 's/^VM_IP=//p')
out="$root/evidence/sprint2/environment/vm-audit.txt"
mkdir -p "$(dirname "$out")"
ssh -o BatchMode=yes -o IdentitiesOnly=yes -o StrictHostKeyChecking=accept-new \
  -o "UserKnownHostsFile=$here/.cache/known_hosts" -i "$key" "stack@$ip" \
  'set -e; date -u +%FT%TZ; cat /etc/os-release; printf "cpu="; nproc; free -h; df -h /opt; printf "virt="; systemd-detect-virt; printf "sudo="; sudo -n true && echo yes; printf "cloud-init="; cloud-init status; printf "repo="; git -C ~/modules-devstack-optimized rev-parse HEAD' > "$out"
cat "$out"
