# Registro de avance observado

Registrar resultados y límites de cada tarea; los planes y criterios viven en [PLANS.md](../PLANS.md). Las instrucciones persistentes están en [AGENTS.md](../AGENTS.md).

## 2026-10-02 — Línea base aceptada de Sprint 2

- **Hecho:** B01–B06 y B19, 7 tareas y 33 puntos. La [matriz de evidencia](../docs/sprint2/sprint2-evidence.md) y la [trazabilidad](../docs/sprint2/final-traceability.md) relacionan las capturas con los criterios.
- **Implementado:** Terraform/Ansible, instalación DevStack, smoke y captura del baseline original. Commits de referencia: `6abd728`, `30491e9`, `6344e3f`.
- **Documentado:** arquitectura/contratos y propuestas Go; informes individuales revisados en `84a8196` y `1032506`.
- **Límite:** el boot ACTIVE corresponde al OpenStack original. No hay sustitución por Go ni benchmark comparativo.
- **Último estado formal del libro derivado:** 7/26 tareas y 33/185 puntos terminados; B21 en progreso y B16 pendiente. Este registro no modifica el Excel.

## 2026-10-26 — Auditoría y adopción del harness

- **Situación encontrada:** había automatización del laboratorio, baselines y gates S2. Faltaban instrucciones de agente en el repositorio, un plan de ejecución recuperable y CI. No había `go.mod`, fuentes `.go` ni pruebas Go. Los wrappers de compatibilidad sólo capturan el original.
- **Fase real:** laboratorio/análisis S2 cerrado; transición a la implementación S3, con B07 aún sin iniciar. El paso del calendario no acredita ejecución.
- **Cambio de esta tarea:** instrucciones persistentes, fases ligadas al backlog, registro de evidencia y verificación local reproducible. El gate distingue código Go ausente de pruebas aprobadas.
- **Verificación previa:** `make docs-check` y `python3 scripts/verify_sprint2.py` aprobados; `terraform fmt -check -recursive infra/lab-vm/terraform` y `ansible-playbook --syntax-check -i localhost, infra/lab-vm/ansible/site.yml` aprobados. Son verificaciones de archivos locales.
- **Laboratorio:** `virsh -c qemu:///system domstate tt-openstack-lab` devolvió `shut off`. No se inició ni aprovisionó la VM.
- **Verificación posterior:** `make check` aprobado: sintaxis/ShellCheck, documentos, evidencia archivada, navegación del harness, 13 pruebas de regresión, formato Terraform y sintaxis Ansible. `make harness-status` confirmó 0 fuentes y 0 tests Go; `make go-check` informó `PENDING Go` y no ejecutó pruebas/build Go. `git diff --check` aprobado.
- **Siguiente tarea funcional:** B07, providers/inventories Go y sus pruebas, según [el alcance definido](../PLANS.md).

Referencia del enfoque: [guía oficial de OpenAI sobre flujos de desarrollo con Codex](https://developers.openai.com/cookbook/examples/codex/iterating-development-workflows-with-codex). Se adapta al repositorio usando su documentación existente y controles ejecutables.

## 2026-10-26 — Verificación continua

- **Cambio:** [GitHub Actions](../.github/workflows/verify.yml) ejecuta `make check` y `make harness-status` en Ubuntu 24.04 para pushes a `main`, PR y ejecución manual. Acciones fijadas por SHA; Terraform fijado en 1.15.9; Go se configura desde `go.mod` cuando exista.
- **Límites:** permisos de lectura, sin secretos del laboratorio, sin arranque de VM ni despliegue. Un workflow fallido informa el fallo; no se configuraron reglas de protección de rama.
- **Verificación local:** YAML parseado y comprobados triggers, permisos, comando y acciones fijadas por SHA. `make check` aprobado nuevamente (13 tests); `git diff --check` aprobado. La VM sigue `shut off`. La ejecución remota sólo se puede confirmar después del push.

## 2026-10-26 — Resultado remoto y compatibilidad del runner

- **Resultado observado:** [CI 40332648391](https://github.com/PaoloCastanon/modules-devstack-optimized/actions/runs/40332648391), commit `036379c`, aprobado en GitHub. Ejecutó los gates locales y omitió explícitamente setup Go porque no existe módulo; no validó servicios Go.
- **Ajuste:** el runner avisó que checkout/setup-terraform usaban Node 20. Se verificaron los `action.yml` oficiales y los tags remotos; checkout v6, setup-terraform v4 y setup-go v6 usan Node 24. Se fijaron sus SHAs para eliminar la dependencia del runtime obsoleto.
- **Verificación local:** YAML y SHAs aprobados; `make check` aprobado (13 tests) y `git diff --check` sin incidencias. El resultado de una ejecución anterior no acredita esta nueva revisión del workflow; consultar el check asociado a su commit en GitHub.

## 2026-10-26 — Revisión PR #1: pin de Nova

- **Revisión:** [PR #1](https://github.com/PaoloCastanon/modules-devstack-optimized/pull/1), HEAD `bbe1d2bdc1aff51a8ee0b6534d5830bee47df00a`. [Informe independiente](../github-pr-1-review.md) conservado como resultado de la revisión; las observaciones describen el estado inspeccionado.
- **Resultado local:** sobre una copia temporal de `main` combinada con el PR, `make check`, `bash tests/compatibility/nova_pin.sh` y `git diff --check HEAD` aprobados. No se detectaron defectos bloqueantes. DevStack fijado admite SHA en `NOVA_BRANCH`.
- **Ajuste del harness:** `make check` incorpora `nova-pin-check` cuando la prueba del PR existe. La ausencia del test se informa explícitamente; no se ejecutan los scripts de captura que contactan OpenStack.
- **Observación de operación:** un `local.conf` antiguo con la rama de Nova debe revisarse antes de reintentar manualmente el instalador: el guard de configuración rechaza diferencias. La reprovisión con marcador completo omite esa instalación. No se modificó configuración ni evidencia de la VM.
- **Límites:** el PR no tiene checks remotos registrados en su HEAD; la verificación anterior es local. No se ejecutó instalación completa ni se inició la VM. La aprobación técnica no equivale a integrar el PR ni a validar una reinstalación.
- **Verificación del ajuste:** `make check` aprobado en main y en el checkout temporal main+PR #1+PR #2; allí ejecutó correctamente el test de Nova. Las 12 pruebas MySQL también aprobaron por separado. `git diff --check` aprobado. Se publicó una revisión APPROVED para el HEAD indicado; el PR sigue abierto, sin merge.

## 2026-10-26 — Revisión PR #2: autenticación MySQL

- **Revisión:** [PR #2](https://github.com/PaoloCastanon/modules-devstack-optimized/pull/2), HEAD `8c84fe87ec30685ed0f8a2dbccf1ac467c3db708`. [Informe independiente](../github-pr-2-review.md) de sólo lectura; los resultados ejecutados por el agente principal se registran aquí.
- **Resultado local:** `make check`, las 12 pruebas `python3 -m unittest discover -s tests/unit -p 'test_mysql_auth.py' -v` y `git diff --check HEAD` aprobados sobre main+PR #2 y sobre main+ambos PR en copias temporales. No se detectaron defectos bloqueantes; se revisó la transición, la idempotencia, los fallos y la protección de credenciales.
- **Ajuste del harness:** `unit-test` descubre `tests/unit` explícitamente desde `make check`. La suite estaba fuera del descubrimiento anterior porque el directorio no es un paquete Python importable. Se informa su ausencia en las revisiones que aún no contienen el PR.
- **Límites:** no hay checks remotos en el HEAD del PR. Las pruebas usan dobles de MySQL/sudo; no se ejecutó una instalación completa con esta revisión exacta. La VM permanece apagada. No se cambió el estado formal del backlog.
- **Verificación del ajuste y aprobación:** `make check` aprobado en main y en el checkout combinado. En este último ejecutó 13 tests del harness, 12 MySQL y la regresión Nova; Terraform/Ansible y `git diff --check HEAD` también aprobados. Se publicó revisión APPROVED sobre el HEAD indicado; el PR permanece abierto, sin merge.

## 2026-10-26 — Integración de los PR #1 y #2

Los estados abiertos indicados en las revisiones anteriores describen ese momento. Ambos PR quedaron posteriormente fusionados, de forma secuencial y con autorización del usuario.

| PR | HEAD actualizado con main | CI aprobado antes del merge | Commit de merge |
|---|---|---|---|
| [#1: pin Nova](https://github.com/PaoloCastanon/modules-devstack-optimized/pull/1) | `57744fe43933f9a4fe5a6821a3e70928b9181aad` | [40336772207](https://github.com/PaoloCastanon/modules-devstack-optimized/actions/runs/40336772207) | `e4baefa7bfa7cbc310f1ad982e06db0baf552447` |
| [#2: auth MySQL](https://github.com/PaoloCastanon/modules-devstack-optimized/pull/2) | `1c6e47adb9e2e465495abb82c1db5e0be0c12023` | [40337050891](https://github.com/PaoloCastanon/modules-devstack-optimized/actions/runs/40337050891) | `652a9ca8e7d81c3a0ae13ae43c3d3676a281d64e` |

- **Procedimiento:** cada rama se actualizó con main; la segunda incorporó también el merge del primer PR. Se comprobó que los archivos funcionales coincidían con las revisiones aprobadas, que main era ancestro y que GitHub informaba `MERGEABLE`, `CLEAN`, aprobación y checks completos sin fallos. Los merges quedaron ligados al SHA comprobado y se confirmó `MERGED` antes de avanzar.
- **Resultado en main:** `make check` aprobado tras ambos merges: 13 pruebas del harness, 12 MySQL, regresión Nova, evidencia archivada, ShellCheck, formato Terraform y sintaxis Ansible. La copia local se actualizó con fast-forward.
- **Documentación:** se corrigió el alcance de versiones fijadas (DevStack y Nova) y se documentó la migración de un `local.conf` anterior al pin, sin cambiar el laboratorio.
- **Límites:** son pruebas de código y archivos; MySQL usa dobles. No se ejecutó instalación completa, no se recapturó evidencia S2 y no se modificó el backlog. `make go-check` sigue informando `PENDING Go`; B07 continúa sin iniciar.
- **VM:** `virsh -c qemu:///system domstate tt-openstack-lab` devolvió `shut off` después de la integración.

## 2026-10-28 — B25, Acción 2/8: filesystem store aislado

- **Implementado:** paquete `internal/glance/store` con inicialización explícita de
  raíz privada, scope UUID y flock; Stage streaming con límite/longitud y MD5/SHA-512;
  Publish con recibo verificado y linkat sin reemplazo; Open seekable con medición
  de integridad; Delete de claves exactas, idempotente y con fsync de directorios.
  No se conectó a cmd/glance ni se implementaron SQL, estados o recuperación global.
- **Seguridad:** aperturas openat/O_NOFOLLOW y verificación del inode; publicación
  desde descriptor mediante procfs. Linux, mismo filesystem, 0700/0600; sin nuevas
  dependencias. El README del paquete documenta garantías, requisitos y límites.
- **Validación local, Go 1.24.4:** gofmt, `go vet -p 1 ./...`, `go build -p 1 ./...`
  y `go mod verify` aprobados. `go test -p 1 -count=1 -json ./...` ejecutó 59
  resultados PASS del store (16 tests y 43 subtests) y 368 PASS de otros paquetes,
  pero falló en el primer test de internal/auth: httptest no pudo abrir un socket
  local (`operation not permitted`). El resto de esa suite no llegó a ejecutarse.
- **Race:** `go test -race -p 1 -count=1 -json ./cmd/glance ./internal/config
  ./internal/glance/...` aprobó esos mismos 427 resultados. No acredita race de
  internal/auth. Se usaron GOMAXPROCS=2, GOTOOLCHAIN=local, GOPROXY=off, GOFLAGS=-p=1
  y GOCACHE privado escribible fuera del repositorio, sin configuración global.
- **Gate completo:** `make check` avanzó por documentación/evidencia, 13 pruebas del
  harness, 12 pruebas MySQL con dobles, regresión Nova y sintaxis/formato de infra;
  falló en go-check por el socket prohibido del mismo test Keystone. No se debilitó
  el gate ni se modificaron las pruebas B20. No se informa make check como aprobado.
- **Límites:** filesystem temporal real y fallos de I/O inyectados están separados;
  ENOSPC es simulado. Reapertura entre instancias no demuestra corte de energía o
  reinicio MySQL. Sin VM, servicios, contenedores, DB real, benchmarks ni integración
  Nova. El gate integral B20 queda pendiente de ejecutarse donde httptest pueda
  abrir sockets; esta acción no cierra B25 ni autoriza la siguiente.

Revalidación del 28/10/2026: se ejecutó `GOMAXPROCS=2 GOTOOLCHAIN=local make check` en `/home/juanc/Proyectos/modules-devstack-main`, con código de salida 0. Pasaron los controles de documentación y evidencias, pruebas Python, verificación Nova, comprobaciones Terraform/Ansible, análisis estático Go, pruebas Go con race detector y compilación. El bloqueo anterior de `httptest` por permisos de sockets correspondía al entorno restringido de la primera ejecución. Esta revalidación no constituye una prueba de integración MySQL real ni de durabilidad ante pérdida de energía.
