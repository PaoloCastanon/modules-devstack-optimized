# Revisión del PR #3 — Glance Go B20

Fecha: 2026-10-28 (America/Santiago).

- PR: [feat(glance): complete B20 Go image metadata catalog](https://github.com/PaoloCastanon/modules-devstack-optimized/pull/3).
- Base: `27d5b77f144b0a8133c702b199c679d87228f3cd`.
- HEAD revisado: `d0d1b3217b5b0982cf73d8872b6a423a6b83da3c`.
- Resultado: **APPROVED**, sin defectos bloqueantes detectados en el alcance B20.
- [Aprobación publicada](https://github.com/PaoloCastanon/modules-devstack-optimized/pull/3#pullrequestreview-5691720702). El PR permanece abierto; no se hizo merge.

## Resumen y riesgo

44 archivos, 6666 líneas añadidas y una eliminada. La revisión fue focalizada:
lectura del código productivo nuevo, contratos, documentación, workflow heredado
y pruebas relevantes. Los componentes de mayor riesgo son Keystone, secretos,
frontera entre proyectos y persistencia transaccional. La aprobación tiene
confianza moderada para ejecución real: no se conectó a MySQL ni Keystone.

| Resultado | Cantidad |
|---|---:|
| Vulnerabilidades críticas o altas identificadas | 0 |
| Defectos bloqueantes identificados | 0 |
| Seguimientos de documentación/cobertura | 2 |

La ausencia de hallazgos no demuestra ausencia de vulnerabilidades.

## Cambios e historia

El rango contiene siete commits (27 de octubre), desde `f177aee` hasta
`d0d1b32`: modelo/configuración, persistencia, auth/API, gates/documentación,
catálogo y merge en el fork. La base no tenía fuentes Go: no hay validaciones
Go previas eliminadas ni regresión de un arreglo de seguridad identificada.
`git blame` sitúa el texto de estado reemplazado del README en `af5fb43`.
El PR no modifica el workflow, los gates existentes, la infraestructura ni S2.

| Área | Revisión |
|---|---|
| `internal/auth`, `internal/config` | Token opaco validado con Keystone, expiración/scope, roles, caché exclusiva del token propio, renovación acotada, cancelación, bloqueo de redirects, lectura privada de secretos y URLs |
| `internal/glance/api` | Autenticación antes del router, límites de cuerpo/query, tipos JSON/null, 401/403, media types, respuestas, esquemas y logs sin tokens/SQL |
| `internal/glance/service`, `model` | Owner derivado de identidad, publicación por admin, privacidad, inmutabilidad, PATCH sin modificar entrada y DELETE de queued/protected |
| `internal/glance/repository/mysql` | Parámetros SQL, identificadores permitidos, visibilidad antes de listar, snapshots, locks, rollback, tombstones, grants aislados y migración explícita con dirty/checksum |
| CLI, Makefile y docs | Inicialización, timeouts, shutdown, selección de toolchain, límites declarados y separación de tests con integración |

## Escenarios adversariales y radio de impacto

Se siguieron las rutas de un cliente sin token, un reader intentando escribir,
un miembro intentando publicar public y un miembro intentando leer/modificar
imágenes private/shared ajenas. Se revisaron fallos Keystone, redirects hacia
otro host, filtros/orden con SQL, fallo en la segunda operación PATCH, fallo SQL
de un hijo y solicitudes concurrentes. Las barreras correspondientes están
implementadas y tienen regresiones locales con dobles.

Conteo de referencias productivas directas por búsqueda estática, excluidos tests:

| Función | Sitios de llamada | Impacto |
|---|---:|---|
| `Keystone.Authenticate` por interfaz | 1 | Todas las rutas HTTP de Glance |
| `Identity.Check` | 7 | Auth y cinco operaciones de servicio |
| `config.ReadSecret` | 2 | DSN y credencial Keystone |
| `config.ValidateURL` | 4 | Configuración y construcción del handler |
| `CheckAccount`, `VerifySchema`, `Migrate` | 1 cada una desde CLI | Arranque o migración |

No hay consumidores Placement implementados en este HEAD. Reutilizar auth
requiere pruebas de políticas Placement; los permisos de Glance no se heredan.

## Verificación ejecutada

Se exportó el HEAD con `git archive` a una carpeta temporal independiente.

| Comando / evidencia | Resultado |
|---|---|
| `make check` sobre HEAD | PASS: 13 harness, 12 MySQL con dobles, regresión pin Nova, ShellCheck, evidencia, Terraform/Ansible, formato Go, vet, race y build |
| `GOTOOLCHAIN=local go test -coverprofile=... ./...` | PASS; cobertura global 84,8% |
| `go tool cover -func=...` | Auth 91,7%; config 91,0%; API 95,6%; modelo 86,8%; MySQL 81,4%; servicio 88,8%; CLI 0% |
| `GOTOOLCHAIN=local go test -tags=integration -run '^$' ./internal/glance/repository/mysql` | PASS de compilación; **cero pruebas de integración ejecutadas** |
| `git diff --check origin/main...origin/pr-3` desde el repositorio | PASS |
| [CI 40613658362](https://github.com/PaoloCastanon/modules-devstack-optimized/actions/runs/40613658362) | completed / success, HEAD revisado |
| `virsh -c qemu:///system domstate tt-openstack-lab` | `shut off` |

Toolchain local observado: `go1.27.3-X:nodwarf5`. El build tag de integración
está fuera de `make check`; el pase de CI tampoco acredita servicios reales.

## Seguimientos y límites

1. **Afirmación de integración inconsistente.** README dice que A y B tienen
   integración real, pero `docs/glance/b20-catalog.md` y el final de
   `docs/glance/development.md` dejan la integración real de B pendiente.
   La descripción del PR no aporta artefactos que resuelvan esa discrepancia.
   Alinear el estado con evidencia reproducible; esta revisión no acepta
   formalmente B20 ni modifica el Excel.
2. **CLI sin cobertura ejecutada.** `cmd/glance/main.go` tiene 0% en esta suite;
   `format_test.go` prueba el Makefile. Añadir una prueba de arranque/cierre y
   errores de dependencias cuando se prepare la siguiente integración operativa.

No se identificaron bloqueos para integrar el catálogo acotado. Continúan fuera
de esta aprobación bytes/store B25, autenticación compuesta de servicio,
discovery completo, membresías, equivalencia negativa completa, Nova y benchmarks.
No se repitió la integración MySQL/Keystone declarada por el autor ni se alteró
el laboratorio. La cobertura con dobles no prueba aislamiento físico, durabilidad
ante cortes ni compatibilidad completa con OpenStack.

Referencias contrastadas: [Image API v2](https://docs.openstack.org/api-ref/image/v2/),
contrato y fixtures Sprint 2. Metodología: skill `differential-review`.
