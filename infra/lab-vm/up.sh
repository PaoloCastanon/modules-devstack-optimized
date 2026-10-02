#!/usr/bin/env bash
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
for cmd in terraform virsh ansible-playbook curl sha256sum; do
  command -v "$cmd" >/dev/null || { echo "Falta $cmd" >&2; exit 1; }
done
[[ -r /dev/kvm ]] || { echo 'KVM no disponible.' >&2; exit 1; }
net_active=$(virsh -c qemu:///system net-info default | awk '$1 == "Active:" {print $2}')
[[ $net_active == yes ]] || { echo 'Red libvirt default no activa.' >&2; exit 1; }
[[ $(nproc) -ge 4 ]] || { echo 'Host con menos de 4 CPU.' >&2; exit 1; }
[[ $(awk '/MemTotal/{print int($2/1024/1024)}' /proc/meminfo) -ge 14 ]] || { echo 'Host con menos de 14 GiB RAM total.' >&2; exit 1; }
[[ $(df -BG --output=avail "$here" | tail -1 | tr -dc '0-9') -ge 65 ]] || { echo 'Host con menos de 65 GiB libres.' >&2; exit 1; }
"$here/fetch-image.sh"
cd "$here/terraform"
terraform init -input=false
terraform fmt -check
terraform validate
terraform plan -input=false -out=lab.tfplan
terraform show -json lab.tfplan | python3 -c '
import json,sys
plan=json.load(sys.stdin)
danger=[c["address"] for c in plan.get("resource_changes",[]) if "delete" in c["change"]["actions"]]
if danger: raise SystemExit("Plan contiene borrados: "+", ".join(danger))
'
terraform apply -input=false lab.tfplan
"$here/provision.sh"
