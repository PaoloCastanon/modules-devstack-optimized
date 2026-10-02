# Pendientes externos y límites al 2026-10-02

No quedan bloqueos para los siete elementos del backlog S2. La VM Ubuntu 24.04.5 está activa; Ansible instaló y verificó DevStack, el smoke test arrancó una instancia hasta `ACTIVE`, y Placement/Glance tienen baseline original en `evidence/sprint2/`.

1. **Retroalimentación de Etapa 1:** no se encontró un informe o comentario formal del profesor guía entre los insumos disponibles. `feedback-etapa1.md` mantiene ese punto pendiente para incorporación manual; no se inventaron observaciones.
2. **Reproducción idéntica a largo plazo:** `versions.lock` fija el SHA de DevStack y registra los SHAs efectivamente instalados de Placement, Glance y Nova. El instalador aún resuelve paquetes y repositorios secundarios al ejecutar. Una réplica bit a bit requeriría fijar también esas dependencias; el laboratorio actual es reproducible funcionalmente mediante Terraform y Ansible.
3. **Pruebas diferenciales Sprint 3:** el baseline cubre las operaciones críticas, pero faltan mutaciones de traits/allocations, errores adicionales y cuerpos HTTP internos exactos de Nova→Placement/Nova→Glance para comparar una futura implementación Go. El log de Placement ya registra rutas, status y microversiones de Nova en el boot real.

Para continuar: usar los fixtures originales como oráculo de compatibilidad e incorporar la retroalimentación formal cuando esté disponible.
