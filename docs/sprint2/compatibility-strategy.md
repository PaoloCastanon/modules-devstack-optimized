# Comparación diferencial Python ↔ Go

Para cada caso, generar una request canónica (método, ruta, query ordenada, headers relevantes, JSON/body hash). Ejecutarla contra servicios originales y Go aislados con el mismo estado inicial. Guardar por separado request sin token, status, headers permitidos, response, snapshot de efectos en DB/store y resultado observado en Nova. `tests/compatibility/*_baseline.sh` reutiliza las capturas originales de Sprint 2; los casos mutantes de traits/allocations se ampliarán en Sprint 3.

Normalizar UUID con mapa estable por rol (`provider-A`, `image-A`), timestamps a marcador, orden de listas sólo si la API no lo garantiza, request IDs y headers volátiles. No normalizar tipos JSON, campos obligatorios, status, errores de dominio ni generation: son parte del contrato. Para binarios comparar hash y longitud; para errores comparar clase/status/código y campos estables. Reportar diferencias con ruta JSON y caso. Verificar efectos tras reinicio. El control end-to-end final es que Nova original cree VM usando Placement Go y Glance Go; se ejecutará en sprints posteriores.

En Sprint 2 todavía no había benchmark ni comparación con Go. El baseline original mide el comportamiento de referencia. [Placement API](https://docs.openstack.org/api-ref/placement/) y [Glance v2](https://docs.openstack.org/api-ref/image/v2/) complementan el contrato observado.

## Ejecución B07 posterior a S2 — 2026-10-28

El [runner vivo](../../scripts/placement_differential.py) ejecutó 38 casos contra
Python fijado y Go, con MySQL aislado y Keystone reales. La
[evidencia](../../evidence/sprint3/placement/b07-live-20261028/README.md) conserva
requests, responses, snapshots por API y diferencias estrictas. Coincidieron
38/38 status y estados antes/después, pero solo 27/38 comparaciones completas;
el comando devolvió 1 por 11 representaciones de error pendientes. No se
normalizaron errores, generation ni tipos JSON para ocultarlas. No hay benchmark
ni boot Nova con Go demostrado por esa ejecución.

## Cierre del alcance B07 — 2026-10-29

La [campaña final](../../evidence/sprint3/placement/b07-close-20261029/README.md)
amplía a 57 casos, con **57/57** status, snapshots y comparaciones estrictas
coincidentes, salida 0 y limpieza verificada. Se conservan el resultado anterior
y el ensayo intermedio que detectó seis diferencias adicionales de prioridad.
No se modificaron las normalizaciones para ocultar errores. [Aceptación B07](../placement/b07-acceptance.md)
limitada a providers/inventories 1.0; el cierre no acredita Nova ni API íntegra.
