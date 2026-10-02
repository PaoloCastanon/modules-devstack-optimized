#!/usr/bin/env bash
set -euo pipefail
name=tt-openstack-lab
virsh -c qemu:///system dominfo "$name"
ip=$(virsh -c qemu:///system domifaddr "$name" --source lease 2>/dev/null | awk '$4 ~ /\// {split($4,a,"/"); print a[1]; exit}')
if [[ -z $ip ]]; then
  ip=$(virsh -c qemu:///system net-dhcp-leases default | awk 'tolower($0) ~ /52:54:00:24:10:02/ {split($5,a,"/"); print a[1]; exit}')
fi
[[ -n $ip ]] || { echo 'IP DHCP todavía no disponible.' >&2; exit 1; }
printf 'VM_IP=%s\n' "$ip"
