# Auditoría y versiones del entorno

El host de trabajo es CachyOS y no se usó para instalar DevStack: [auditoría del host](../../evidence/sprint2/environment/host-audit.txt). El laboratorio dedicado `tt-openstack-lab` corre Ubuntu 24.04.5 en libvirt, con 4 vCPU, 8 GiB asignados y disco de 55 GiB; la [auditoría de VM](../../evidence/sprint2/environment/vm-audit.txt) registra CPU, RAM, disco, sudo y cloud-init. Terraform crea la VM y Ansible instala/valida DevStack como `stack`.

[versions.lock](../../infra/devstack/versions.lock) fija DevStack `stable/2026.2` SHA `0fb9685c1bf22cd0b4d576d7a74f64195ad934fe` y registra los SHAs desplegados de Placement (`f4a89d3…`), Glance (`a3e5f49…`) y Nova (`247ea5d…`). El sistema usa Python 3.12.3, MySQL 8.0.46 y OpenStackClient 10.3.0. Ansible clona el commit exacto de este repositorio, que contiene el parche de inicio de Neutron Geneve; el instalador aplica ese parche al SHA fijado antes de `stack.sh`.

Los servicios críticos se verificaron con OpenStackClient y sus [respuestas](../../evidence/sprint2/devstack/service-list.json). `make lab-provision` se repitió tras una instalación correcta y volvió a validar los endpoints sin reinstalar. El repositorio no incluye `.env`, `local.conf` ni logs crudos, que pueden contener secretos. Los SHAs de proyectos secundarios y paquetes Python dependen de lo resuelto por el upstream en la fecha de instalación; para reproducir un entorno idéntico deben inmovilizarse también esos repositorios y dependencias.

Fuente de plataforma: [DevStack Quick Start](https://docs.openstack.org/devstack/latest/) recomienda Ubuntu 24.04 y máquina dedicada.
