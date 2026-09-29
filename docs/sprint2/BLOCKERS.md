# Bloqueos reales al 2026-09-29

1. **B01/B02/OE2:** el host auditado es CachyOS, `systemd-detect-virt=none`, y no tiene sudo no interactivo. [Auditoría](../../evidence/sprint2/environment/host-audit.txt). DevStack advierte que modifica sustancialmente el host y debe instalarse en servidor o VM dedicados. Se requiere Ubuntu 24.04 dedicado, 4 CPU, 8 GiB RAM, 40 GiB libres, red y sudo no interactivo. No se ejecutó `stack.sh`.
2. **B03/B04/B05/B06/B19:** no existe OpenStack local operativo. No se pueden comprobar endpoints, microversión máxima *del servidor desplegado*, tráfico Nova, backend Glance, smoke test ni boot de VM. La inspección upstream no sustituye baseline real.
3. **Retroalimentación:** no se halló informe o comentarios formales del profesor en los archivos aportados; se requiere aportarlos para completar ese apartado.

Para desbloquear: crear VM Ubuntu 24.04 dedicada; configurar `infra/devstack/.env` desde el ejemplo; ejecutar `make devstack-setup`, `make devstack-verify`, `make smoke`, `make baseline`; revisar resultados y actualizar el Excel sólo con evidencia real.
