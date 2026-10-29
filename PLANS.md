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

## Placement: B07 y B08 completados en el alcance seleccionado

**Cierre técnico al 2026-10-29:** [B07 aceptado para el alcance priorizado
providers/inventories 1.0](docs/placement/b07-acceptance.md). La campaña final dio
**57/57** coincidencias estrictas, status y estados iguales contra Python fijado,
con MySQL/Keystone reales, reinicio de ejecutable, concurrencia y limpieza
verificados. [Evidencia y ensayos previos](evidence/sprint3/placement/b07-close-20261029/README.md).
El PR #6 ya fue integrado en b4d8300.

**B08 al 2026-10-29:** [catálogos y asociaciones classes/traits](docs/placement/b08-development.md)
implementados con migración v3, 21 clases y 377 traits estándar, CUSTOM, borrados
protegidos y generation. Las rutas seleccionadas incorporan los cambios 1.1–1.7:
**82/82** comparaciones estrictas y regresión B07 **57/57**, MySQL/Keystone reales,
reinicio y limpieza comprobados. [Evidencia B08](evidence/sprint3/placement/b08-close-20261029/README.md).
La asociación positiva de aggregates sólo se verificó en MySQL aislado.

**B09 completado para las rutas priorizadas:** [entrega A](docs/placement/b09-development.md) implementa
allocations atómicas, consultas por consumer/provider, usages y capacidad restante,
con migración v4. **60/60** casos vivos, B07 **57/57** y B08 **82/82**; rollback,
concurrencia y reinicio verificados. El [cierre B09](evidence/sprint3/placement/b09-close-20261029/README.md) añade
formato 1.12, generación 1.28 y mappings 1.34 para PUT 1.36: **24/24** casos
modernos estrictos. El rango alto se limita a allocations hasta ampliar los
perfiles del resource tracker en B10.
[Evidencia B09-A](evidence/sprint3/placement/b09a-20261029/README.md).

**En ejecución por solicitud del usuario:** B10 candidates y cierre B11/B12. B11/B12 requieren cierre
transversal. El alcance sigue siendo el subconjunto para Nova; no se acredita
sustitución Nova, toda la API 1.8 ni se cambia el Excel histórico.

El [plan Placement B07–B15](docs/placement/implementation-plan.md) conserva el
orden. Los resultados anteriores de 27/38 y ocho diferencias pendientes describen
etapas intermedias; el build log conserva esa historia sin sustituir capturas.

### Estado histórico previo a implementar B07

**Estado al revisar el repositorio el 2026-10-26:** sin iniciar. No existe `go.mod` ni fuentes `.go`. La infraestructura sí está implementada y tiene evidencia de Sprint 2; la reimplementación de los servicios aún no comenzó.

### Entradas de B07

- [Arquitectura](docs/sprint2/go-target-architecture.md), [modelo Placement](docs/sprint2/placement-data-model.md), [ADR-01 y ADR-03](docs/sprint2/design-decisions.md).
- [Contrato de rutas y microversiones](docs/sprint2/placement-api-scope.md) y [estrategia de pruebas](docs/sprint2/test-strategy.md).
- [Capturas originales Placement](evidence/sprint2/placement/). Completar casos que aún no tienen fixture; no inventar respuestas de Nova.

### Alcance de B07

Reutilizar el módulo Go integrado y completar el recorrido API→servicio→repositorio de providers/inventories, con descubrimiento y negociación de las microversiones necesarias. El almacenamiento sigue ADR-01; no escribir en la DB original. Auth de producción se verifica transversalmente en B12: cualquier identidad de prueba debe quedar restringida a tests, sin un bypass desplegable.

### Verificación y aceptación de B07

1. **Prueba inicial:** definir casos de create/list/show/update/delete, validación, duplicados y errores de generation según el contrato original. Registrar el comportamiento que falta antes de implementarlo.
2. **Implementación:** hacer pasar esos casos con la separación de capas prevista y persistencia consistente; B11 completa y verifica los requisitos transversales de persistencia y reinicio.
3. **Refactor:** mantener el contrato y volver a ejecutar las pruebas.
4. **Verificación local:** `make check`; los tests Go deberán ejecutarse, sin salida de pendiente.
5. **Compatibilidad:** ejecutar casos equivalentes sobre original y Go, conservando estado inicial, request, status, campos estables y efectos. Las pruebas locales aisladas no cierran la integración Nova de B13–B15.

La [aceptación B07](docs/placement/b07-acceptance.md) registra resultados, tests, casos diferenciales y límites en el build log. No se considera completado por crear carpetas, un health endpoint o respuestas HTTP constantes.

## Comandos y límites

- `make check`: verificación local del harness, sintaxis/lint, infraestructura, evidencia archivada y Go cuando exista. No contacta el laboratorio.
- `make harness-status`: inventario actual de fuentes y referencia al estado aceptado.
- `make evidence-check`: valida sólo las capturas archivadas de S2; no indica disponibilidad actual de servicios.
- `make go-check`: con código Go, exige módulo, pruebas de los dominios presentes, formato, vet, race detector y build. Sin código, informa explícitamente que la implementación está pendiente.
- `make lab-status`: consulta libvirt; los comandos de instalación, smoke y baseline se ejecutan sólo para tareas que requieran el laboratorio.

El [workflow de trabajo](AGENTS.md) conserva evidencia y registra los comandos ejecutados antes de cada commit. Las verificaciones locales no reemplazan la sustitución real con Nova ni los benchmarks.
