#!/usr/bin/env bash
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
root=$(cd "$here/../.." && pwd)
key=${LAB_SSH_KEY:-$HOME/.ssh/id_ed25519}
[[ -f $key ]] || { echo "Falta clave SSH privada: $key" >&2; exit 1; }
mkdir -p "$here/.cache"
ip=$("$here/status.sh" | sed -n 's/^VM_IP=//p')
revision=$(git -C "$root" rev-parse HEAD)
ssh_opts=(-o BatchMode=yes -o IdentitiesOnly=yes -o StrictHostKeyChecking=accept-new -o "UserKnownHostsFile=$here/.cache/known_hosts" -i "$key")
ready=0
for attempt in {1..30}; do
  if ssh "${ssh_opts[@]}" -o ConnectTimeout=5 "stack@$ip" true >/dev/null 2>&1; then ready=1; break; fi
  printf 'Esperando SSH en %s (%s/30)\n' "$ip" "$attempt" >&2
  sleep 10
done
((ready)) || { echo "SSH no respondió en $ip después de 5 minutos." >&2; exit 1; }
ansible-playbook -i "$ip," -u stack --private-key "$key" \
  --ssh-common-args "-o IdentitiesOnly=yes -o StrictHostKeyChecking=accept-new -o UserKnownHostsFile=$here/.cache/known_hosts" \
  --extra-vars "lab_management_ip=$ip repo_revision=$revision" \
  "$here/ansible/site.yml"
scp "${ssh_opts[@]}" "stack@$ip:/home/stack/modules-devstack-optimized/infra/devstack/versions.lock" "$root/infra/devstack/versions.lock"
mkdir -p "$root/evidence/sprint2/devstack"
snapshot=$(mktemp -d "$here/.cache/verification.XXXXXX")
trap 'rm -f "$snapshot"/*; rmdir "$snapshot"' EXIT
scp "${ssh_opts[@]}" "stack@$ip:/home/stack/modules-devstack-optimized/evidence/sprint2/devstack/*.json" "$snapshot/"
scp "${ssh_opts[@]}" "stack@$ip:/home/stack/modules-devstack-optimized/evidence/sprint2/devstack/token-expiry.txt" "$snapshot/"
for file in "$snapshot"/*; do
  target="$root/evidence/sprint2/devstack/${file##*/}"
  if [[ ${LAB_REFRESH_EVIDENCE:-0} == 1 || ! -e $target ]]; then cp "$file" "$target"; fi
done
echo "DevStack preparado y verificado: ssh -i $key stack@$ip"
