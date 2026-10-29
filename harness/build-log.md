# Registro de avance observado

Registrar resultados y límites de cada tarea; los planes y criterios viven en [PLANS.md](../PLANS.md). Las instrucciones persistentes están en [AGENTS.md](../AGENTS.md).

## 2026-10-28 — B07: primer recorrido Placement Go persistente

- **Base:** PR #3 integrado por el usuario en `85bae56`. Se conservaron y reaplicaron los dos commits locales de revisión/plan sobre esa base; trabajo funcional en `feat/placement-b07`.
- **Criterio del incremento:** CRUD providers y lectura/reemplazo inventories mediante HTTP→servicio→MySQL, con roles Placement, errores y control de generation. Se escribieron primero tests de modelo/servicio y API; fallaron por ausencia de implementación (`NewProvider`, interfaces y handler `New`) antes de añadir el código.
- **Contrato contrastado:** fixtures S2 y handlers/schemas/objetos/policies upstream fijados en `f4a89d3`. POST provider 1.0 devuelve 201 vacío; PUT provider no acepta ni incrementa generation; reemplazar inventories hace CAS e incrementa una vez. Se anuncia únicamente 1.0 y tres clases observadas; [guía de alcance y límites](../docs/placement/b07-development.md).
- **Implementado:** `cmd/placement`, capas `internal/placement`, migración v1 y guardias para esquema/cuentas Placement separados. Se reutiliza `internal/auth`; `internal/config` comparte el loader con namespace Placement independiente y conserva la interfaz Glance. No hay bypass de auth desplegable; root discovery permanece público según upstream. Se corrigió la afirmación contradictoria del README sobre integración real de Glance B.
- **Prueba real MySQL:** instancia descartable 8.0.46 en contenedor con puerto solo loopback y datos tmpfs; schemas nuevos y cuentas migrador/runtime separadas por suite. Migración repetida, rechazo de cuenta DDL para HTTP, duplicados, generation obsoleta, rollback tras fallo SQL del segundo hijo, concurrencia, reapertura de pool, borrado y dirty: PASS. El primer ensayo observó código FK 1216, no solo 1452; ambos se traducen y el rollback se comprobó en un esquema nuevo.
- **Ejecutable real:** suite compila el binario y ejecuta migrate/serve; creación HTTP con adaptador Keystone, SIGTERM, reinicio del proceso y consulta del estado persistido: PASS. La prueba corrigió su reutilización de un contexto HTTP que podía expirar durante shutdown; el request posterior usa su propio plazo. Keystone es un servidor HTTP de fixture en ambas suites, no el servicio desplegado.
- **Verificación final:** `make check` PASS, incluidos formato/vet/race/build Go para Glance y Placement y los gates preexistentes; tests históricos API/404 PASS; `make placement-test-mysql` PASS; `make placement-test-binary` PASS; `make placement-build` PASS; `git diff --check` PASS. [Salidas saneadas](../evidence/sprint3/placement/b07/README.md). La comparación 404 omite solo request_id; las capturas S2 no se modificaron.
- **Límites y siguiente paso:** B07 en progreso, con B11/B12 parciales. Falta comparación en vivo Python↔Go, completar negativos/microversiones y verificar Keystone real/Nova. Sin allocations, no se acredita inventario en uso; B09 debe añadir sus invariantes antes de reservar. No se reinició el servidor MySQL ni se probó corte de energía. No se cambió el Excel ni se declaró boot ACTIVE con Go.
- **Limpieza:** se eliminó la instancia descartable con sus datos y archivos de credenciales temporales al terminar. La VM `tt-openstack-lab` permaneció `shut off`; no se inició ni reaprovisionó.

## 2026-10-28 — Revisión y aprobación PR #3: Glance B20

- **Revisión:** [PR #3](https://github.com/PaoloCastanon/modules-devstack-optimized/pull/3), HEAD `d0d1b3217b5b0982cf73d8872b6a423a6b83da3c`, sobre base `27d5b77`. [Informe](../github-pr-3-review.md) con alcance, cobertura y límites.
- **Pruebas sobre copia aislada del HEAD:** `make check` PASS, incluidos Go vet/race/build; `go test -coverprofile=... ./...` PASS, 84,8% global. Compilación con `-tags=integration -run '^$'` PASS, sin ejecutar integración. `git diff --check origin/main...origin/pr-3` PASS desde el repo.
- **Remoto:** [CI 40613658362](https://github.com/PaoloCastanon/modules-devstack-optimized/actions/runs/40613658362) success. [Review APPROVED](https://github.com/PaoloCastanon/modules-devstack-optimized/pull/3#pullrequestreview-5691720702) publicada con SHA explícito mediante `gh` después de que el conector devolviera 403 por permisos. El PR sigue abierto, sin merge.
- **Límites:** no se detectaron defectos bloqueantes. README y documentación discrepan sobre la integración real de B; el CLI no tiene cobertura de statements. No se conectó a MySQL/Keystone, no se validó Nova ni se modificó el estado formal B20/Excel. La base main local todavía no incorpora código Go de este PR.
- **VM:** `virsh -c qemu:///system domstate tt-openstack-lab` devolvió `shut off`; no se inició.

## 2026-10-28 — Plan de reimplementación Placement

- **Propuesto:** [plan B07–B15](../docs/placement/implementation-plan.md), enlazado desde PLANS.md. Prioriza contrato y providers/inventories, con persistencia B11 y auth B12 desde el primer recorrido; luego traits/classes, allocations, candidates y sustitución Nova. No se escribió código Placement ni se modificó el Excel.
- **Hallazgos de contrato:** la traza Nova incluye GET allocations por provider, ausente de la tabla propuesta; candidates requiere root_required y versión 1.36. La API oficial distingue POST provider 201 vacío antes de 1.20 y 200 con cuerpo después. El plan exige resolver estos casos y los del resource tracker antes de asumir compatibilidad.
- **Dependencia:** PR #3 aprobado pero sin merge. Las piezas Go reutilizables existen en su HEAD; main local no contiene ese código.
- **Verificación local de los documentos de revisión y plan:** `make check` PASS y `git diff --check` PASS. En main, Go sigue explícitamente PENDING por ausencia de fuentes; el pase Go de esta sesión corresponde únicamente a la copia aislada del PR #3. No se inició el laboratorio.

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
