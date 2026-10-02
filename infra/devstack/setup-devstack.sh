#!/usr/bin/env bash
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
log="$here/setup-devstack.log"
config_tmp=''
on_exit() {
  rc=$?
  if [[ -n $config_tmp ]]; then rm -f "$config_tmp"; fi
  if ((rc)); then
    if [[ -f $log ]]; then printf 'Instalación fallida (%s). Log: %s\n' "$rc" "$log" >&2
    else printf 'Preflight falló (%s); stack.sh no se ejecutó.\n' "$rc" >&2; fi
  fi
}
trap on_exit EXIT
[[ $EUID -ne 0 ]] || { echo 'Ejecutar como usuario no root.' >&2; exit 1; }
source /etc/os-release
[[ ${ID:-} == ubuntu && ${VERSION_ID:-} == 24.04 ]] || { echo 'Se requiere VM/servidor dedicado Ubuntu 24.04.' >&2; exit 1; }
[[ -f "$here/.env" ]] || { echo "Copie .env.example a $here/.env y configure secretos." >&2; exit 1; }
# shellcheck disable=SC1091
source "$here/.env"
[[ ${DEDICATED_LAB:-NO} == YES ]] || { echo 'DEDICATED_LAB=YES sólo en VM/servidor dedicado.' >&2; exit 1; }
for cmd in git curl sed tee sudo python3 systemd-detect-virt; do command -v "$cmd" >/dev/null || { echo "Falta $cmd" >&2; exit 1; }; done
sudo -n true || { echo 'Se requiere sudo no interactivo.' >&2; exit 1; }
for var in ADMIN_PASSWORD DATABASE_PASSWORD RABBIT_PASSWORD SERVICE_PASSWORD; do
  value=${!var:-}; [[ $value =~ ^[A-Za-z0-9]{12,}$ && $value != CHANGE_ME ]] || { echo "Configure $var con >=12 caracteres alfanuméricos." >&2; exit 1; }
done
[[ $(nproc) -ge 4 ]] || { echo 'Se requieren >=4 CPU.' >&2; exit 1; }
[[ $(awk '/MemTotal/{print $2}' /proc/meminfo) -ge 8000000 ]] || { echo 'Se requieren al menos 8 GiB asignados (>=8000000 KiB visibles).' >&2; exit 1; }
[[ $(df -BG --output=avail /opt | tail -1 | tr -dc '0-9') -ge 40 ]] || { echo 'Se requieren >=40 GiB libres en /opt.' >&2; exit 1; }
curl -fsS --max-time 10 -o /dev/null https://opendev.org || { echo 'Sin red a opendev.org.' >&2; exit 1; }
[[ $(systemd-detect-virt) == kvm || $(systemd-detect-virt) == qemu ]] || { echo 'Sólo se instala en VM KVM/QEMU dedicada.' >&2; exit 1; }
if [[ ${HOST_IP:-} == AUTO_OR_CHANGE_ME ]]; then HOST_IP=$(ip -4 route get 1.1.1.1 | awk '{for(i=1;i<=NF;i++) if($i=="src") print $(i+1)}'); fi
[[ $HOST_IP =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo 'HOST_IP IPv4 inválido.' >&2; exit 1; }
# shellcheck disable=SC1091
source "$here/versions.lock"
dir=${DEVSTACK_DIR:-/opt/stack/devstack}
[[ $dir == /opt/stack/devstack ]] || { echo 'DEVSTACK_DIR debe ser /opt/stack/devstack para este laboratorio.' >&2; exit 1; }
sudo install -d -o "$(id -un)" -g "$(id -gn)" /opt/stack /opt/stack/logs
if [[ ! -d $dir/.git ]]; then git clone --branch "$DEVSTACK_BRANCH" https://opendev.org/openstack/devstack "$dir"; fi
[[ $(git -C "$dir" remote get-url origin) == https://opendev.org/openstack/devstack ]] || { echo 'Origen DevStack inesperado.' >&2; exit 1; }
git -C "$dir" fetch origin "$DEVSTACK_BRANCH"
git -C "$dir" checkout --detach "$DEVSTACK_SHA"
[[ $(git -C "$dir" rev-parse HEAD) == "$DEVSTACK_SHA" ]] || exit 1
patch_file="$here/patches/neutron-geneve-startup.patch"
if ! git -C "$dir" apply --reverse --check "$patch_file" 2>/dev/null; then
  git -C "$dir" apply --check "$patch_file"
  git -C "$dir" apply "$patch_file"
fi
conf="$dir/local.conf"
config_tmp=$(mktemp /opt/stack/local.conf.XXXXXX)
cp "$here/local.conf.template" "$config_tmp"
chmod 600 "$config_tmp"
for var in ADMIN_PASSWORD DATABASE_PASSWORD RABBIT_PASSWORD SERVICE_PASSWORD HOST_IP; do
  value=${!var}; sed -i "s/@$var@/$value/g" "$config_tmp"
done
if [[ -e $conf ]]; then
  cmp -s "$config_tmp" "$conf" || { echo "Ya existe $conf con distinta configuración; revisar manualmente." >&2; exit 1; }
else
  mv "$config_tmp" "$conf"
  config_tmp=''
fi
printf '\nIntento %s: DevStack %s (%s), SO %s\n' "$(date -u +%FT%TZ)" "$DEVSTACK_BRANCH" "$DEVSTACK_SHA" "$PRETTY_NAME" | tee -a "$log"
# A failed prior run can leave uWSGI listening on an obsolete port while
# stack.sh rewrites Apache's proxy target. Start retries with fresh services.
mapfile -t prior_units < <(systemctl list-unit-files 'devstack@*.service' --no-legend --no-pager | awk '{print $1}')
if ((${#prior_units[@]})); then
  sudo systemctl stop "${prior_units[@]}"
fi
if (cd "$dir" && ./stack.sh) >> "$log" 2>&1; then
  rc=0
else
  rc=$?
fi
((rc==0)) || exit "$rc"
printf 'Instalación correcta. Log: %s\n' "$log"
"$here/record-versions.sh"
