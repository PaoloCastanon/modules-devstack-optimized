# modules-devstack-optimized

Trabajo de Título: análisis y futura reimplementación acotada de OpenStack Placement y Glance en Go. El [estado de Sprint 2](docs/sprint2/README.md), la [matriz de evidencia](docs/sprint2/sprint2-evidence.md) y el [informe](docs/sprint2/informe_etapa2.pdf) distinguen diseño preparado de validación real.

`make docs-check` valida artefactos locales; `make sprint2-verify` exige evidencia de DevStack, baseline y boot y falla hasta que exista. La [guía de laboratorio](infra/devstack/README.md) es para una VM Ubuntu 24.04 dedicada. No ejecutar DevStack en la estación de trabajo.
