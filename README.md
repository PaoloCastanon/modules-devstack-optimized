# modules-devstack-optimized

Trabajo de Título: análisis y futura reimplementación acotada de OpenStack Placement y Glance en Go. El [estado de Sprint 2](docs/sprint2/README.md), la [matriz de evidencia](docs/sprint2/sprint2-evidence.md) y el [informe](docs/sprint2/informe_etapa2.pdf) distinguen diseño preparado de validación real.

`make docs-check` valida artefactos locales; `make sprint2-verify` exige evidencia de DevStack, baseline y boot y falla hasta que exista. La [guía de laboratorio](infra/devstack/README.md) es para una VM Ubuntu 24.04 dedicada. No ejecutar DevStack en la estación de trabajo.

La [Entrega A de Glance Go](docs/glance/development.md) incorpora creación y consulta
de metadatos, repositorio MySQL y autenticador Keystone. Su guía distingue tests
aislados, integración pendiente y límites; no representa B20 completo ni sustitución
del servicio original. No requiere ejecutar `lab-up`, `smoke` o `baseline`.

La [Entrega B del catálogo](docs/glance/b20-catalog.md) añade listado paginado,
JSON Patch, borrado lógico y esquemas de metadatos. Sus pruebas de integración
SQL se seleccionan por separado y requieren un esquema nuevo autorizado;
los resultados reales de la Entrega A no validan automáticamente estas operaciones.
