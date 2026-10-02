#!/usr/bin/env bash
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
cache="$here/.cache"
image="$cache/ubuntu-24.04-server-cloudimg-amd64.img"
expected=$(cat "$here/image.sha256")
mkdir -p "$cache"
if [[ -f $image ]]; then
  printf '%s  %s\n' "$expected" "$image" | sha256sum --check --status || { echo 'La imagen en caché no coincide con el SHA fijado.' >&2; exit 1; }
  echo "Imagen verificada: $image"
  exit 0
fi
url=https://cloud-images.ubuntu.com/releases/noble/release/ubuntu-24.04-server-cloudimg-amd64.img
curl --fail --location --retry 3 --continue-at - --output "$image.part" "$url"
printf '%s  %s\n' "$expected" "$image.part" | sha256sum --check --status || { echo 'SHA256 de imagen Ubuntu incorrecto.' >&2; exit 1; }
mv "$image.part" "$image"
echo "Imagen verificada: $image"
