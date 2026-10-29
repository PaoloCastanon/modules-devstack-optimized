# modules-devstack-optimized

Trabajo de Título: análisis y futura reimplementación acotada de OpenStack Placement y Glance en Go. El [estado de Sprint 2](docs/sprint2/README.md), la [matriz de evidencia](docs/sprint2/sprint2-evidence.md) y el [informe](docs/sprint2/informe_etapa2.pdf) distinguen diseño preparado de validación real.

## Estado y forma de trabajo

El laboratorio y el análisis de Sprint 2 están aceptados. El catálogo Glance Go B20 (Entregas A y B) está implementado; su alcance y validación se detallan en la documentación de cada entrega. B25 tiene componentes y [arranque operativo](docs/glance/b25-operations.md) implementados con regresiones locales y suites opt-in para comprobar MySQL 8/002 y la integración binaria. Placement [B07 está implementado y validado](docs/placement/b07-acceptance.md) para providers/inventories 1.0, con MySQL/Keystone reales y 57/57 comparaciones estrictas; su entrega fue integrada mediante el PR #6. [B08](docs/placement/b08-development.md) añade catálogos y asociaciones classes/traits, con 82/82 comparaciones estrictas y regresión B07 57/57. El [plan por fases](PLANS.md), las [instrucciones para agentes](AGENTS.md) y el [registro de resultados](harness/build-log.md) permiten continuar con contexto y criterios verificables.

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

Ambas entregas tienen pruebas locales. La documentación acredita integración real
para A y mantiene pendiente la validación operativa de las nuevas rutas B.
La transferencia binaria y su arranque operativo se implementan en B25;
su guía separa las regresiones locales de las verificaciones reales.
La integración Nova y la comparación de rendimiento no se acreditan
mediante los controles locales.

## Placement Go — B07/B08 completos; B09 en progreso

La [guía B07](docs/placement/b07-development.md) describe CRUD de providers,
inventories con generation, esquema MySQL aislado y autenticación Keystone.
Incluye fixtures S2, suites opt-in con MySQL real y [campaña de cierre](evidence/sprint3/placement/b07-close-20261029/README.md):
**57/57** respuestas estrictas, status y estados coinciden con Placement Python
y Keystone reales. [B08](docs/placement/b08-development.md) añade catálogos estándar
y CUSTOM, traits por provider y cambios seleccionados 1.1–1.7: **82/82**
comparaciones estrictas, upgrade v3 y persistencia tras reinicio.
[B09-A](docs/placement/b09-development.md) añade allocations atómicas, usages y
capacidad restante, con **60/60** comparaciones estrictas. Default 1.0, latest 1.8;
el rango se limita a las rutas seleccionadas. B09 sigue en progreso: el formato
y la generación de consumer que usa Nova aún faltan; candidates e integración
Nova continúan pendientes. La asociación positiva de aggregates
se verificó en MySQL aislado, fuera de la comparación viva.
[Alcance aceptado y límites](docs/placement/b07-acceptance.md).
`make placement-build` compila el ejecutable; `make check` verifica ambos módulos
sin iniciar el laboratorio. La guía distingue los resultados reales y los fixtures
de identidad utilizados en las pruebas.
