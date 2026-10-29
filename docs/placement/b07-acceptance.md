# B07 — aceptación técnica del alcance priorizado

Fecha: 2026-10-29. Responsable Placement: Paolo Castañon Barrera.

**B07 completado e implementado para providers/inventories 1.0.** El código,
pruebas y evidencia están publicados en el [PR #6](https://github.com/PaoloCastanon/modules-devstack-optimized/pull/6);
su integración en main queda pendiente del merge. No se modifica el Excel ni
se cierra B11/B12, la integración Nova o el resto de Placement.

El criterio original de B07 es «Operaciones priorizadas de providers e
inventories superan las pruebas definidas» (libro original, Backlog!A10/H10).
La [referencia de lectura y hash](../../evidence/sprint3/placement/b07-close-20261029/planning-reference.json)
conserva ese criterio sin alterar el archivo. La selección sigue la tabla B07
del [plan](implementation-plan.md) y la [guía implementada](b07-development.md).

## Criterios y evidencia

| Criterio | Resultado observado |
|---|---|
| CRUD providers, lookup y filtros name/uuid | Create 201 vacío/Location; list/show/rename 200; DELETE 204 y posterior 404; conflictos de nombre/UUID 409 |
| Inventories completos e individuales | Lectura, defaults, reemplazo y vaciado; generación correcta; error de clase y capacidad con estado intacto |
| Errores y concurrencia | JSON Schema, tipo/contenido, generación obsoleta, permisos y prioridades reproducidos; CAS/rollback/concurrencia verificados en MySQL |
| Persistencia real y ejecutable | Migración explícita/idempotente, cuentas separadas, reapertura de pool, SIGTERM/reinicio de proceso y mismo provider/inventory/generation |
| Autenticación | Adaptador productivo contra Keystone desplegado; roles admin/service positivos, reader 403, sin token e inválido 401 |
| Compatibilidad diferencial | **57/57** status, respuestas estrictas y snapshots antes/después coinciden con Python fijado; runner termina en **0** y toda limpieza queda verificada |
| Regresiones locales | make check PASS: formato, vet, race y build para Glance y Placement; oráculo adicional de 61 casos de esquema y fixtures HTTP originales |

[Campaña completa, comandos, procedencia y límites](../../evidence/sprint3/placement/b07-close-20261029/README.md).
Los 57 casos conservan los 38 anteriores y añaden 19 comprobaciones de tipos,
límites, orden, prioridades y Accept. El ensayo ampliado intermedio detectó seis
diferencias adicionales; se conservó su salida 1 y se corrigieron antes del cierre.

## Decisiones de contrato

- Se anuncia únicamente **1.0**, incluyendo default y latest. Las versiones
  posteriores se rechazan; las regresiones locales cubren límites y cabeceras
  inválidas. No se anuncia 1.36 por haber verificado sólo estas rutas.
- La validación se ejecuta en Go con los cuatro esquemas 1.0 inspeccionados.
  El orden de objetos/números se conserva para los errores; el normalizador
  decimal de generation sigue evitando redondeos. No hay intérprete Python,
  llamadas al original ni fixtures en la ejecución productiva.
- Content-Type precede a la política de la ruta; un PUT comprueba existencia
  antes del body, y generation antes de capacidad/clase. La capacidad se valida
  en orden de entrada antes de resolver clases. El repositorio conserva su lock
  y CAS final; la lectura previa no sustituye la comprobación transaccional.
- Las consultas extra para verificar existencia/generation priorizan el contrato.
  No se acredita mejora de rendimiento; los benchmarks corresponden a su tarea.

## Fronteras del cierre

El alcance es el subconjunto explícito de operaciones y pruebas de B07, no toda
la API de Placement ni todos los posibles cuerpos/errores de Python. Sólo se
admiten las clases VCPU, MEMORY_MB y DISK_GB observadas; B08 completa catálogos y
traits. No hay allocations, candidates, árboles o aggregates implementados.
Los links heredados a usages no añaden esa ruta al subconjunto Go.

Se mantienen límites locales de body/query, profundidad JSON 128 e int64 para
generation. No se acredita equivalencia de todas las combinaciones de HTML,
Accept, Unicode extremo o números fuera de esos límites. El formato de errores
está ligado al Python 3.12/jsonschema 4.26.0/WebOb 1.8.11 inspeccionados.

El reinicio probado es de proceso/pool, no reinicio del servidor MySQL ni corte
de energía. Expiración/revocación real y cierre transversal de seguridad siguen
en B12. No se cambiaron endpoints de Nova, configuración del original, evidencia
S2 ni informes académicos. La VM volvió a shut off y se retiraron recursos y
credenciales temporales.

**Siguiente tarea: B08**, catálogos y asociaciones de resource classes/traits.
Las ampliaciones de microversión se fijarán con evidencia del resource tracker
y sus pruebas antes de anunciarlas; allocations y candidates continúan en B09/B10.
