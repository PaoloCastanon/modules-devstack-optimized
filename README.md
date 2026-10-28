# modules-devstack-optimized

Trabajo de Título: análisis y futura reimplementación acotada de OpenStack Placement y Glance en Go. El [estado de Sprint 2](docs/sprint2/README.md), la [matriz de evidencia](docs/sprint2/sprint2-evidence.md) y el [informe](docs/sprint2/informe_etapa2.pdf) distinguen diseño preparado de validación real.

## Estado y forma de trabajo

El laboratorio y el análisis de Sprint 2 están aceptados. Glance Go B20 (Entregas A y B) está implementado y validado; Placement B07 sigue pendiente. B25 tiene componentes y [arranque operativo](docs/glance/b25-operations.md) implementados con regresiones locales; su validación MySQL 8/002 y Keystone/binarios reales sigue pendiente. El [plan por fases](PLANS.md), las [instrucciones para agentes](AGENTS.md) y el [registro de resultados](harness/build-log.md) permiten continuar con contexto y criterios verificables.

```bash
make check           # Verificación local; no inicia la VM ni contacta OpenStack
make harness-status  # Inventario Go y referencia al estado aceptado
```

`make check` requiere Python 3, Make, Bash, ShellCheck, Terraform y Ansible. Ejecuta sintaxis/lint de scripts, navegación del harness, pruebas de los gates, evidencia S2 archivada, formato Terraform y sintaxis Ansible. Cuando exista código Go, exige también Go/gofmt, formato, vet, tests con race detector y build. Sin código Go, muestra `PENDING Go`; pasar los controles locales no acredita servicios reimplementados.

También ejecuta explícitamente la suite `tests/unit` y la regresión aislada `tests/compatibility/nova_pin.sh` cuando estén presentes; si faltan en la revisión actual, lo informa. Las pruebas MySQL requieren PyYAML y usan dobles: no conectan a la VM ni a una base de datos.

`make docs-check` conserva la validación documental existente. `make evidence-check` valida las capturas históricas de DevStack, baseline y boot. `make sprint2-verify` puede verificar servicios vivos si encuentra `OS_AUTH_URL` u openrc: usarlo sólo en el laboratorio. La [guía de laboratorio](infra/devstack/README.md) es para una VM Ubuntu 24.04 dedicada. No ejecutar DevStack en la estación de trabajo.

La VM permanece apagada mientras no se use. Los informes de S2 reflejan la entrega del 2 de octubre; las verificaciones locales posteriores no prueban disponibilidad actual del laboratorio.

El [workflow de CI](.github/workflows/verify.yml) ejecuta los mismos controles en pushes a `main`, pull requests y ejecución manual. Usa permisos de lectura y no requiere credenciales del laboratorio. Un fallo marca el check como fallido; configurar protección de rama en GitHub es una tarea administrativa aparte.

## Glance Go — B20

La [Entrega A](docs/glance/development.md) implementa creación y consulta
de metadatos con MySQL y autenticación Keystone.

La [Entrega B](docs/glance/b20-catalog.md) añade listado paginado,
JSON Patch, borrado lógico y esquemas de metadatos.

Ambas entregas cuentan con validación local e integración real.
La transferencia de imágenes, integración Nova y comparación de
rendimiento permanecen pendientes.
