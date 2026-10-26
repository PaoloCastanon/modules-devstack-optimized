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
