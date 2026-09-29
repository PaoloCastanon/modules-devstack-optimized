# Comparación diferencial Python ↔ Go

Para cada caso, generar una request canónica (método, ruta, query ordenada, headers relevantes, JSON/body hash). Ejecutarla contra servicios originales y Go aislados con el mismo estado inicial. Guardar por separado request sin token, status, headers permitidos, response, snapshot de efectos en DB/store y resultado observado en Nova. `tests/compatibility/*_baseline.sh` prepara fixtures de lectura; los casos mutantes se agregarán cuando exista laboratorio.

Normalizar UUID con mapa estable por rol (`provider-A`, `image-A`), timestamps a marcador, orden de listas sólo si la API no lo garantiza, request IDs y headers volátiles. No normalizar tipos JSON, campos obligatorios, status, errores de dominio ni generation: son parte del contrato. Para binarios comparar hash y longitud; para errores comparar clase/status/código y campos estables. Reportar diferencias con ruta JSON y caso. Verificar efectos tras reinicio. El control end-to-end final es que Nova original cree VM usando Placement Go y Glance Go; se ejecutará en sprints posteriores.

No hay benchmark ni compatibilidad medida todavía. [Placement API](https://docs.openstack.org/api-ref/placement/) y [Glance v2](https://docs.openstack.org/api-ref/image/v2/) son fuentes del contrato preliminar.
