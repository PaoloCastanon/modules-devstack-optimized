# Instalación reproducible

1. Crear VM/servidor dedicado Ubuntu 24.04 con al menos 4 vCPU, 8 GiB RAM, 40 GiB libres, acceso a Internet y usuario no root con sudo no interactivo. Revisar [README de infraestructura](../../infra/devstack/README.md).
2. `cp infra/devstack/.env.example infra/devstack/.env`; reemplazar los cuatro secretos con valores alfanuméricos únicos de 12 o más caracteres, definir `HOST_IP` o dejar detección automática y fijar `DEDICATED_LAB=YES` sólo tras comprobar dedicación.
3. `make devstack-setup`. El script valida plataforma, recursos y red, clona DevStack y fija SHA antes de `stack.sh`. El log completo se guarda ignorado por Git en `infra/devstack/setup-devstack.log`; `local.conf` con secretos queda en `/opt/stack/devstack/local.conf` fuera del repositorio.
4. `make devstack-verify`, `make smoke`, `make baseline`. Revisar evidencias sanitizadas y registrar SHA/versiones mediante `record-versions.sh` (se ejecuta automáticamente al finalizar instalación).

El `local.conf.template` conserva el grafo de servicios por defecto de DevStack, que incluye Keystone, Nova, Placement, Glance, Neutron, MySQL y RabbitMQ. [Configuración oficial](https://docs.openstack.org/devstack/latest/configuration.html), [servicios por defecto](https://docs.openstack.org/devstack/latest/). La versión exacta de dependencias queda pendiente hasta instalación; no se presume que un branch garantice los commits de todos los proyectos.
