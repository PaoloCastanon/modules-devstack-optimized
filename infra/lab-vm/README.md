# VM DevStack con Terraform + Ansible

Se crea **sólo** `tt-openstack-lab` en `qemu:///system` y el pool `default` de libvirt. Perfil: Ubuntu 24.04, 4 vCPU, 8192 MiB RAM, disco qcow2 sparse de 55 GiB, SSH por clave al usuario `stack` con sudo no interactivo. La VM queda aislada de la estación de trabajo. Terraform no modifica las otras redes, pools ni dominios existentes.

La interfaz de administración usa `192.168.122.10/24` en la red `default` de libvirt. Otra interfaz usa la red de usuario de QEMU (`10.0.2.15` por DHCP) como ruta de salida y DNS. Esto permite acceso a Internet incluso cuando el firewall del host bloquea el tráfico reenviado por la red NAT de libvirt. El host debe tener libre `192.168.122.10` en esa red.

## Uso

Desde la raíz del repo: `make lab-up`; ejecuta descarga con SHA256 fijo, `terraform init/plan/apply`, espera SSH y corre `ansible/site.yml`. Luego `make lab-verify` deja auditoría en `evidence/sprint2/environment/vm-audit.txt`. `make lab-status` muestra el dominio y la IP de administración. Se usa `~/.ssh/id_ed25519` y su `.pub` por defecto; cambiar con `LAB_SSH_KEY` y `TF_VAR_ssh_public_key_path` si se necesita.

Terraform fija `dmacvicar/libvirt` 0.9.9 mediante `.terraform.lock.hcl`. La imagen oficial Ubuntu 24.04 se descarga desde `https://cloud-images.ubuntu.com/releases/noble/release/ubuntu-24.04-server-cloudimg-amd64.img` y se valida contra `image.sha256`. La caché, el estado y los planes se excluyen de Git. Fuente: [imagen Ubuntu para libvirt](https://ubuntu.com/docs/public-images/public-images-how-to/launch-with-libvirt/), [provider libvirt](https://registry.terraform.io/providers/dmacvicar/libvirt/latest/docs).

Ansible verifica el SO, instala herramientas base y el agente QEMU, crea `/opt/stack` y clona el repo en `/home/stack/modules-devstack-optimized`. No instala DevStack ni guarda contraseñas. El siguiente paso dentro de la VM es crear `infra/devstack/.env` a partir del ejemplo y ejecutar `make devstack-setup` allí. Consultar [guía de instalación](../../docs/sprint2/devstack-installation.md).

La VM **no** arranca automáticamente con el host. Para apagarla sin destruirla: `virsh -c qemu:///system shutdown tt-openstack-lab`. Para volver a encenderla: `virsh -c qemu:///system start tt-openstack-lab`. `terraform destroy` elimina la VM y sus volúmenes; revisar el plan y respaldar datos antes de usarlo.
