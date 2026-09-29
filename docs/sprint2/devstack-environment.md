# Auditoría y versiones del entorno

La auditoría reproducible está en [host-audit.txt](../../evidence/sprint2/environment/host-audit.txt): CachyOS rolling, usuario `paolo`, 16 CPU, 15 GiB RAM, 82 GiB libres, red a opendev.org, sin virtualización detectada ni sudo no interactivo. Es una estación de trabajo; no se instalará DevStack aquí.

Objetivo del laboratorio: Ubuntu 24.04 dedicado, un nodo, DevStack `stable/2026.2` en commit `0fb9685c1bf22cd0b4d576d7a74f64195ad934fe` verificado con `git ls-remote` el 2026-09-29. Los commits de Placement, Glance, Nova y las versiones de Python, MariaDB/MySQL y OpenStackClient **instalados** figuran `UNINSTALLED` en [versions.lock](../../infra/devstack/versions.lock) y `record-versions.sh` los registrará tras una instalación correcta. Los clones upstream inspeccionados para diseño tienen SHAs en `evidence/sprint2/*/upstream-source-audit.txt`, pero no son versiones desplegadas.

Fuente de plataforma: [DevStack Quick Start](https://docs.openstack.org/devstack/latest/) recomienda Ubuntu 24.04 y advierte que debe usarse una máquina dedicada.
