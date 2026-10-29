# Plan de implementación

Este plan organiza el backlog existente; no reemplaza el Excel ni modifica sprints o puntos. Los resultados observados pertenecen al [build log](harness/build-log.md). La arquitectura y los contratos permanecen en [la documentación de Sprint 2](docs/sprint2/README.md).

## Fases y aceptación

| Fase | Backlog / sprint | Criterio para considerarla terminada |
|---|---|---|
| Laboratorio y análisis | B01–B06 y B19 / S2 | Instalación reproducible, smoke con boot ACTIVE, contratos y capturas del original. Evidencia aceptada: [matriz S2](docs/sprint2/sprint2-evidence.md). |
| Placement Go | B07–B12 / S3; B21 transversal | Providers/inventories, traits/classes, allocations, candidates, persistencia y auth dentro del alcance; pruebas unitarias/API, casos de error y comparación con el original. |
| Integración y Glance Go | B13–B16, B20, B22, B25 / S4 | Nova usa Placement Go y arranca una instancia; integración automatizada; método experimental definido; Glance gestiona catálogo y bytes con pruebas de integridad. |
| Sustitución Glance y evaluación | B17, B18, B23, B24, B26 / S5 | Nova arranca desde Glance Go; mediciones repetidas de latencia/throughput y CPU/RAM con metodología fija; resultados y memoria final. |

Las fases son agrupaciones de aceptación, no una prohibición de avanzar en paralelo en los dos módulos. B26 conserva S5. B21 continúa en progreso y B16 pendiente según la última aceptación; preparar estos documentos no los cierra.

## Próxima unidad de implementación: B07

**Incremento al 2026-10-29:** corregidos y reproducidos contra capturas los errores
reserved-total, unsupported-media y unacceptable; [verificación](evidence/sprint3/placement/b07-errors-20261029/README.md)
local y MySQL real aprobadas. Quedan ocho diferencias históricas de JSON Schema
sin resolver. No se repitió la campaña Python↔Go/Keystone real; sus cifras del
28 de octubre se conservan como resultado histórico, no como pase del código nuevo.

**Ejecución actualizada al 2026-10-28:** [Placement Go B07–B15](docs/placement/implementation-plan.md). El PR #3 ya está integrado en `85bae56`; se reutilizan módulo y auth Go. [B07](docs/placement/b07-development.md) implementa providers/inventories 1.0, persistencia y auth. La comparación con Python y Keystone reales dio 38/38 status y estados observables iguales, pero 27/38 coincidencias estrictas: quedan 11 representaciones de error por resolver. B07 sigue en progreso; falta cerrar esas diferencias y completar el contrato necesario para Nova. B11/B12 tienen avance parcial, sin cierre transversal. El estado histórico siguiente describe la revisión del 26 de octubre.

**Estado al revisar el repositorio el 2026-10-26:** sin iniciar. No existe `go.mod` ni fuentes `.go`. La infraestructura sí está implementada y tiene evidencia de Sprint 2; la reimplementación de los servicios aún no comenzó.

### Entradas

- [Arquitectura](docs/sprint2/go-target-architecture.md), [modelo Placement](docs/sprint2/placement-data-model.md), [ADR-01 y ADR-03](docs/sprint2/design-decisions.md).
- [Contrato de rutas y microversiones](docs/sprint2/placement-api-scope.md) y [estrategia de pruebas](docs/sprint2/test-strategy.md).
- [Capturas originales Placement](evidence/sprint2/placement/). Completar casos que aún no tienen fixture; no inventar respuestas de Nova.

### Alcance de B07

Reutilizar el módulo Go integrado y completar el recorrido API→servicio→repositorio de providers/inventories, con descubrimiento y negociación de las microversiones necesarias. El almacenamiento sigue ADR-01; no escribir en la DB original. Auth de producción se verifica transversalmente en B12: cualquier identidad de prueba debe quedar restringida a tests, sin un bypass desplegable.

### Verificación y aceptación

1. **Prueba inicial:** definir casos de create/list/show/update/delete, validación, duplicados y errores de generation según el contrato original. Registrar el comportamiento que falta antes de implementarlo.
2. **Implementación:** hacer pasar esos casos con la separación de capas prevista y persistencia consistente; B11 completa y verifica los requisitos transversales de persistencia y reinicio.
3. **Refactor:** mantener el contrato y volver a ejecutar las pruebas.
4. **Verificación local:** `make check`; los tests Go deberán ejecutarse, sin salida de pendiente.
5. **Compatibilidad:** ejecutar casos equivalentes sobre original y Go, conservando estado inicial, request, status, campos estables y efectos. Las pruebas locales aisladas no cierran la integración Nova de B13–B15.

Para aceptar B07, registrar los resultados, la ruta de los tests, los casos diferenciales y las diferencias pendientes en el build log. No se considera completado por crear carpetas, un health endpoint o respuestas HTTP constantes.

## Comandos y límites

- `make check`: verificación local del harness, sintaxis/lint, infraestructura, evidencia archivada y Go cuando exista. No contacta el laboratorio.
- `make harness-status`: inventario actual de fuentes y referencia al estado aceptado.
- `make evidence-check`: valida sólo las capturas archivadas de S2; no indica disponibilidad actual de servicios.
- `make go-check`: con código Go, exige módulo, pruebas de los dominios presentes, formato, vet, race detector y build. Sin código, informa explícitamente que la implementación está pendiente.
- `make lab-status`: consulta libvirt; los comandos de instalación, smoke y baseline se ejecutan sólo para tareas que requieran el laboratorio.

El [workflow de trabajo](AGENTS.md) conserva evidencia y registra los comandos ejecutados antes de cada commit. Las verificaciones locales no reemplazan la sustitución real con Nova ni los benchmarks.
