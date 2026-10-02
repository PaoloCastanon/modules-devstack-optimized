#!/usr/bin/env bash
set -euo pipefail
name=tt-openstack-lab
virsh -c qemu:///system dominfo "$name"
[[ $(virsh -c qemu:///system domstate "$name") == running ]] || { echo 'VM apagada.' >&2; exit 1; }
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
ip=$(terraform -chdir="$here/terraform" output -raw management_ip)
printf 'VM_IP=%s\n' "$ip"
