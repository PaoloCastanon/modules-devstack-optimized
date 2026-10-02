# Comparación diferencial Python ↔ Go

Para cada caso, generar una request canónica (método, ruta, query ordenada, headers relevantes, JSON/body hash). Ejecutarla contra servicios originales y Go aislados con el mismo estado inicial. Guardar por separado request sin token, status, headers permitidos, response, snapshot de efectos en DB/store y resultado observado en Nova. `tests/compatibility/*_baseline.sh` reutiliza las capturas originales de Sprint 2; los casos mutantes de traits/allocations se ampliarán en Sprint 3.

Normalizar UUID con mapa estable por rol (`provider-A`, `image-A`), timestamps a marcador, orden de listas sólo si la API no lo garantiza, request IDs y headers volátiles. No normalizar tipos JSON, campos obligatorios, status, errores de dominio ni generation: son parte del contrato. Para binarios comparar hash y longitud; para errores comparar clase/status/código y campos estables. Reportar diferencias con ruta JSON y caso. Verificar efectos tras reinicio. El control end-to-end final es que Nova original cree VM usando Placement Go y Glance Go; se ejecutará en sprints posteriores.

No hay benchmark ni comparación con Go todavía. El baseline original ya mide el comportamiento de referencia. [Placement API](https://docs.openstack.org/api-ref/placement/) y [Glance v2](https://docs.openstack.org/api-ref/image/v2/) complementan el contrato observado.
