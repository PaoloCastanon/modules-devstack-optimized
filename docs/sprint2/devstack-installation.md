# Instalación reproducible

1. En el host con KVM y libvirt, ejecutar `make lab-up`. Terraform levanta la VM dedicada Ubuntu 24.04; Ansible verifica SO, recursos, virtualización, sudo y red, genera secretos únicos sólo dentro de la VM y ejecuta `setup-devstack.sh` como `stack`.
2. Ansible fija el commit del repositorio del instalador, usa el SHA de DevStack en `versions.lock`, espera `stack.sh` y comprueba Keystone, Nova, Placement y Glance. Una marca de instalación se crea sólo después de verificar los servicios; `make lab-provision` revalida el entorno y reintenta una instalación fallida.
3. El log crudo se conserva en la VM en `/home/stack/modules-devstack-optimized/infra/devstack/setup-devstack.log`; `.env` y `/opt/stack/devstack/local.conf` contienen secretos y no se versionan. El host recibe `versions.lock` actualizado y JSON de verificación sin tokens.
4. Ejecutar los smoke tests y baseline desde la VM: `ssh -i ~/.ssh/id_ed25519 stack@192.168.122.10`, luego `cd ~/modules-devstack-optimized && make smoke && make baseline`. Revisar las evidencias antes de cambiar estados del backlog.

Para una VM dedicada distinta de este laboratorio, también se puede usar el instalador manual: crear `infra/devstack/.env` desde `.env.example`, fijar los secretos, `HOST_IP` y `DEDICATED_LAB=YES`, y ejecutar `make devstack-setup` como usuario no root con sudo sin contraseña.

El `local.conf.template` conserva el grafo de servicios por defecto de DevStack, que incluye Keystone, Nova, Placement, Glance, Neutron, MySQL y RabbitMQ. [Configuración oficial](https://docs.openstack.org/devstack/latest/configuration.html), [servicios por defecto](https://docs.openstack.org/devstack/latest/). La versión exacta de dependencias queda pendiente hasta instalación; no se presume que un branch garantice los commits de todos los proyectos.
