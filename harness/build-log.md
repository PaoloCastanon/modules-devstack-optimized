# Registro de avance observado

Registrar resultados y límites de cada tarea; los planes y criterios viven en [PLANS.md](../PLANS.md). Las instrucciones persistentes están en [AGENTS.md](../AGENTS.md).

## 2026-10-29 — B07: cierre técnico del alcance priorizado 1.0

- **Criterio y alcance:** B07, «Operaciones priorizadas de providers e inventories superan las pruebas definidas» (consulta de sólo lectura al libro original, Backlog!H10). [Aceptación técnica](../docs/placement/b07-acceptance.md): providers/inventories 1.0 implementados y validados; PR #6 pendiente de merge. El Excel y los informes S2 permanecen intactos. No se confunde este cierre con B08–B12 o Nova.
- **Regresión previa:** ocho errores de esquema reproducidos contra una exportación intacta de `4d1976c`, con salida 1. La validación nueva usa cuatro esquemas upstream 1.0 embebidos, JSON con orden/lexemas y diagnósticos compatibles con jsonschema/WebOb inspeccionados; producción enteramente Go. Se contrastó un oráculo adicional de 61 casos sintéticos y se preservó el parser decimal exacto.
- **Prioridades:** se amplió el runner de 38 a 57 casos, sin cambiar canonical/differences ni relajar campos. El ensayo intermedio de 53 casos tuvo 47 coincidencias estrictas, 48 status y 53 estados, salida 1: seis diferencias de prioridad. Se corrigieron existencia antes del body, generation antes de capacidad/clase, capacidad antes de resolver clases, orden de entrada y Content-Type antes de política; escrituras conservan la semántica Accept original. Lock/CAS final permanecen en MySQL.
- **Campaña final real:** [57/57 status, respuestas estrictas y estados iguales](../evidence/sprint3/placement/b07-close-20261029/README.md), runner salida **0**, toda limpieza de providers verificada. Python `f4a89d3`, MySQL Go 8.0.46 aislado y Keystone desplegado. Usuario/proyecto temporales, application credentials con expiración y roles admin/service/reader separados; validador sólo service. Proxy temporal loopback monta /placement, sin modificar bodies/responses ni original/Nova.
- **Persistencia:** `make placement-test-mysql`, `make placement-test-binary` y `make placement-test-upgrade` PASS con race, tres esquemas nuevos independientes por ensayo. Además, binario con Keystone real conserva provider/inventory/generation después de SIGTERM y reinicio; canario eliminado y 404 comprobado. No se reinicia el servidor MySQL ni se prueba corte de energía.
- **Gate y regresiones:** `make check` PASS, incluyendo ambos dominios, formato/vet/race/build; API con race y casos de snapshots/headers PASS; regresión de orden/Accept repetida 30 veces PASS. Capturador del oráculo con SHA/versiones exigidos produce el mismo fixture; sintaxis Python y `git diff --check` PASS. [Fuentes/hash del candidato probado](../evidence/sprint3/placement/b07-close-20261029/attempt-3/provenance.json).
- **Limpieza:** se retiraron canarios/providers, application credentials, usuario/proyecto y archivos privados; ausencia verificada. Procesos/proxies y contenedores MySQL descartables retirados. `tt-openstack-lab` volvió a `shut off`; [pruebas de limpieza](../evidence/sprint3/placement/b07-close-20261029/keystone-cleanup.json).
- **Siguiente:** B08 classes/traits, sin anunciar versiones superiores hasta cubrir sus contratos. Permanecen fuera del cierre API íntegra, HTML/todas las combinaciones de negociación, Unicode/números fuera de límites locales, allocations/candidates, cierre transversal B11/B12, Nova y benchmarks.

## 2026-10-29 — B07: errores de negociación y capacidad

- **Base y criterio:** `git pull --ff-only` dejó main al día en `dfd7624`; incremento en `fix/placement-b07-errors`. Reproducir exactamente las tres respuestas Python archivadas reserved-total, unsupported-media y unacceptable, manteniendo estado y headers estables.
- **Reproducción previa:** la nueva prueba sobre una exportación de main falla en los tres casos con salida 1, antes de modificar el código productivo. [Evidencia antes/después](../evidence/sprint3/placement/b07-errors-20261029/README.md).
- **Cambios:** 406 por Accept insatisfecho usa texto UTF-8 y cuerpo capturado, separado del 406 de microversión; 415 conserva el media type real; el modelo distingue capacidad inválida y el servicio añade UUID/clase al detalle sin escrituras. Se conserva 1.0, SQL/migraciones, auth y el comparador.
- **Verificación local:** `make check` PASS (Glance y Placement, formato/vet/race/build); `go test -race ./internal/placement/... -count=1` PASS; regresiones dirigidas PASS con cuerpos/headers originales, invariancia de estado y variantes de UUID/clase/capacidad/media type. `git diff --check` PASS.
- **MySQL real:** `make placement-test-mysql` PASS, 8.0.46 aislado en tmpfs con puerto loopback, esquema nuevo y migrador/runtime separados; nuevos rechazos HTTP para VCPU/MEMORY_MB conservan inventario/generation por lectura SQL. Keystone fixture. El primer intento de aprovisionamiento falló antes de tests por un readiness prematuro; se conservó el log y el reintento exigió SELECT 1 autenticado por TCP. Se retiraron contenedores/datos/credenciales al terminar.
- **Límites:** no se repitió la campaña en vivo con Python/Keystone real ni Nova; las cifras 27/38 y 11 diferencias describen el código del 28 de octubre. Tres diferencias se corrigen contra capturas; quedan ocho de JSON Schema. B07 continúa en progreso. Sin cambios a Excel/S2. VM apagada durante toda esta tarea.

## 2026-10-29 — Revisión e integración del PR #5: Glance B25

- **Revisión:** [PR #5](https://github.com/PaoloCastanon/modules-devstack-optimized/pull/5), HEAD `fbb4a3d24bdd8b102e2e24c1a7b052e1563a41b5` sobre `0279987`. [Informe](../github-pr-5-review.md): auth compuesta, aislamiento de configuración, streaming, filesystem, intentos SQL, recuperación, DELETE y migraciones. Sin defectos bloqueantes detectados.
- **Verificación del HEAD aislado:** `make check` PASS (vet/race/build incluidos); `go test -coverprofile=/tmp/pr5-coverage.out ./...` PASS, 79,6% global; `git diff --check origin/main...origin/pr-5` PASS. [CI 40876937535](https://github.com/PaoloCastanon/modules-devstack-optimized/actions/runs/40876937535) success en el mismo SHA.
- **MySQL real:** 8.0.46 descartable, tmpfs y puerto loopback; tres esquemas nuevos separados. `TestB25MySQLVersionedUpgrade`, `TestB25MySQLConcurrentFences` y `TestB25MySQLFilesystemHTTP` con `-tags=integration -race -count=1`: PASS. El pipeline verifica bytes, recibo persistido, reapertura y borrado físico con migrador/runtime separados; auth usa doble. Se retiraron contenedor, datos y credenciales privadas al terminar.
- **Remoto:** [APPROVED](https://github.com/PaoloCastanon/modules-devstack-optimized/pull/5#pullrequestreview-5698455549) ligado al HEAD. Con autorización del usuario, merge `4c07cb5fcc1b023b3f607ffc9f3788f40176f41f`, confirmado merged. Se usó `gh` autenticado para escritura ante la limitación de permisos conocida del conector. main local actualizado por fast-forward; su árbol coincide exactamente con el HEAD probado.
- **Verificación posterior:** `make check` PASS sobre main integrado con el informe y este registro; `git diff --check` PASS. No se modificó código funcional durante la revisión.
- **Límites:** no se repitió Keystone real ni boot Nova. Los resultados CLI/Nova del cuerpo del PR pertenecen al autor; falta incorporar/enlazar evidencia N3–N7 y alinear documentación operativa. Siguen pendientes los casos compuestos reales de expiración/revocación, instalación independiente y fallos físicos. No se cambió Excel, backlog formal ni S2. VM `tt-openstack-lab`: `shut off`.

## 2026-10-28 — B07: comparación viva y correcciones de contrato

- **Criterio del incremento:** comparar el subconjunto 1.0 contra el Python fijado con identidad real, observar negativos y efectos, corregir diferencias funcionales y conservar las de representación sin simular un pase.
- **Runner implementado:** `scripts/placement_differential.py`, con tests de no coerción, normalización limitada, archivos privados y protección de recursos existentes. Cada escenario usa UUID/nombre nuevos; preflight exige ausencia, finally elimina únicamente esos UUID y verifica 404. No sigue redirects ni guarda tokens. El gate local usa dobles; el runner vivo es explícito y mutante.
- **Primer ensayo sobre `1e95402`:** 34 casos, 32 status y 9 comparaciones estrictas coincidentes; limpieza verificada. Diferencias observadas: enteros `3.0`, query repetida, collation de nombres, precisión FLOAT, UUID de query y errores. Se reprodujeron fallos en tests antes de corregirlos.
- **Cambios:** parser entero decimal exacto para no redondear generation; último query value en 1.0; UUID de path case-insensitive y query canónico; shape 401 de Keystone y varios errores estables. La migración **v2** cambia igualdad/uniqueness de nombre y FLOAT; v1 y su checksum permanecen intactos. La lectura CAST a CHAR conserva el número textual que devuelve MySQL al driver Python.
- **Laboratorio real:** se inició la VM existente y se verificó Placement `f4a89d3`, MySQL y Keystone; no hubo reprovisión. Se creó un proyecto/usuario temporal con credenciales de aplicación de roles limitados y expiración. Go validó tokens admin/service/reader mediante Keystone desplegado; MySQL Go corrió en contenedor descartable separado, con cuentas app/migrador de esquema propio. No se cambiaron endpoints de Nova ni la DB/config del original.
- **Resultado final:** [38 casos](../evidence/sprint3/placement/b07-live-20261028/README.md), **38/38 status**, **38/38 snapshots antes/después**, **27/38 coincidencias estrictas**. Diez diferencias permanecen en detail de validación y una en body/Content-Type de 406. El comparador devolvió **1**, no PASS; todas las limpiezas de providers quedaron verificadas. Las capturas S2 se conservaron.
- **Persistencia:** upgrade real v1→v2 conservó un canario y generation. La suite de colisión comprobó que dos nombres de v1 que colisionan en v2 dejan dirty y conservan ambas filas; serve rechaza v1 incompleto y no se repara dirty automáticamente. No se probó corte de energía ni reinicio del servidor MySQL.
- **Verificación:** `make check` PASS (Glance y Placement: formato, vet, race, build; harness y 16 tests Python unit); `make placement-test-mysql` PASS; `make placement-test-binary` PASS; `TestV1CollisionStopsUpgrade` con race PASS; `git diff --check` PASS. Las suites locales de integración siguen usando Keystone fixture, distinto de la matriz real. [Metadatos y evidencia](../evidence/sprint3/placement/b07-live-20261028/provenance.json).
- **Limpieza:** providers, application credentials, usuario/proyecto temporales, contenedor MySQL y archivos privados retirados. Se comprobó ausencia del usuario/proyecto y que `tt-openstack-lab` volvió a `shut off`.
- **Pendiente:** cerrar las 11 diferencias estrictas, ampliar microversiones según Nova y sus pruebas. B07 sigue en progreso; B11/B12 parciales. No se modificó el Excel ni se declaró integración/boot Nova.

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

## 2026-10-28 — B25, Acción 6/8: HTTP binario autenticado inyectable

- **Implementado:** PUT/GET `/v2/images/{id}/file` mediante opción explícita de
  construcción del handler, reutilizando Keystone B20 y UploadCoordinator. PUT
  valida octet-stream, declaraciones y stream real; GET exige active/committed,
  coteja tamaño/MD5/SHA-512 antes de cabeceras y transmite con buffer fijo.
  Incluye rangos únicos 206, rechazo 400/416, 204 sin contenido y 503 para blob
  confirmado perdido/corrupto. Después de cabeceras, aborta errores sin anexar JSON.
- **Seguridad:** recuperación obligatoria también para download, capacidad
  compartida y cierre de descriptores antes de liberar propiedad. Se distingue
  fallo de fuente (400) de fallo filesystem (503) sin exponer diagnósticos.
  No cambia cmd/glance: scope SQL/store, recuperación al arrancar y deadlines de
  transferencia siguen pendientes de Acción 8; DELETE coordinado, de Acción 7.
- **Compatibilidad:** se leyeron image_data.py y wsgi.py del commit Python fijado
  a3e5f490b7a28b1161714c94acffe5308d24b567. Se confirmó el MD5 completo también
  para 206, PUT 204, media type 415 y rangos múltiples 400/incorrectos 416.
  Las diferencias y límites están en `docs/glance/b25-http.md`; no se ejecutó Python.
- **Validación local Go 1.24.4:** gofmt; pruebas focalizadas API (203 PASS: 32 tests
  y 171 subtests) y servicio (185 PASS: 33 tests y 152 subtests), sin FAIL/SKIP;
  `go vet -p 1 ./...`, `go mod verify` y `make check` con código 0. Se usaron
  GOMAXPROCS=2, GOTOOLCHAIN=local, GOFLAGS=-p=1 y GOPROXY=off. El gate completo
  incluye documentación/evidencias, 25 pruebas Python, Nova, Terraform/Ansible,
  pruebas Go con race detector y compilación. No se alteraron gates ni tests previos.
- **Límites:** httptest y dobles de SQL/identidad; un recorrido local utiliza
  coordinador y filesystem reales sobre directorios temporales. No acredita
  integración MySQL 8/002, Keystone binario, reinicio ni durabilidad física. Sin
  VM, contenedores, servicios originales, DDL real, Git de escritura o Acción 7/8.

## 2026-10-28 — B25, Acción 7/8: DELETE coordinado y recuperación

- **Implementado:** DELETE del recurso imagen mediante el mismo coordinador
  exclusivo de PUT/GET. TombstoneData bloquea imagen antes de operación, repite
  política/protected y confirma tombstone + cleanup/deleted en una transacción.
  Conserva UUID, recibos, propiedades y tags; queued sin operación no inventa
  intentos. Relee SQL antes de Store.Delete y después de CompleteDataCleanup.
  Un resultado incierto no acredita rollback ni permite unlink a ciegas.
- **Concurrencia:** DELETE y uploads se excluyen durante toda su ejecución.
  Un writer vivo produce 503 controlado sin tombstone ni I/O destructivo; esta
  opción autorizada en Acción 7 difiere de la cancelación prevista en el diseño.
  Se comprueban intentos tardíos, dos DELETE, Close ocupado y GET con fd abierto,
  que puede terminar tras unlink en Linux. No se mantiene SQL durante filesystem.
- **Recuperación:** bajo arranque exclusivo, cleanup/deleted termina en tombstone
  cleaned y cleanup/killed confirmado termina en killed. Reintenta Delete/fsync
  si los nombres ya desaparecieron; no promueve saving ni limpia active válido.
  Inventario obsoleto, estados contradictorios o dependencias fallidas conservan
  evidencia y bloquean admisión. No usa TTL, barridos ni reconstrucciones.
- **Validación local Go 1.24.4:** gofmt; MySQL 280 PASS (32 tests/248 subtests),
  servicio 257 PASS (43/214), API 227 PASS (38/189): 764 resultados, sin FAIL/SKIP.
  `go vet -p 1 ./...` y `make check` terminaron con código 0, incluido race ./...,
  compilación, documentación/evidencias, 25 Python, Nova y Terraform/Ansible.
  GOMAXPROCS=2, GOTOOLCHAIN=local, GOFLAGS=-p=1, GOPROXY=off.
- **Desarrollo:** el primer caso terminal falló porque su fixture conservaba
  campos calculados de active al simular saving/cleanup; se corrigió ese fixture,
  manteniendo el rechazo de estados contradictorios. La negativa previa se
  conserva para killed/cleanup incompatible y se añade el caso válido positivo.
- **Límites:** SQL y auth simulados; HTTP/coordinador con filesystem temporal real.
  TestMySQLCoordinatedDeletion se preparó y compiló con tag integration y run
  vacío, pero NO SE EJECUTÓ: requiere esquema nuevo independiente y autorización
  posterior. No demuestra MySQL/Keystone reales, reinicio de servicios ni pérdida
  de energía. Se leyó DELETE Python del commit fijado; no se ejecutó el original.
  000/001/002 y store intactos. Sin VM, contenedores, DDL real, infraestructura,
  Git de escritura ni Acción 8. Protocolo y divergencias: docs/glance/b25-deletion.md.

## 2026-10-28 — B25, Acción 8/8: arranque operativo y regresiones locales

- **Implementado:** cmd/glance conecta B25 mediante opt-in de configuración;
  verifica cuenta runtime y esquema sin migrar, vincula server_uuid/esquema con
  scope/marcador, adquiere flock y completa RecoverUploads antes de escuchar.
  store-scope es lectura; store-init exige root existente vacío e inicialización
  explícita. No hay reparación, esquema/root alternativo o bypass de auth.
- **Lifecycle:** plazos de metadatos, upload/download, SQL, resolución, recovery y
  shutdown separados. StopAdmission + WaitIdle conservan ownership de writers,
  readers y DELETE. Shutdown cancela conexiones al vencer la gracia y espera
  handlers/compensación antes de cerrar filesystem y SQL. Un fsync del kernel
  no interrumpible puede prolongar esa espera; no se afirma cierre limpio en ese caso.
- **Regresión encontrada durante esta acción:** limpiar deadlines al retornar
  el handler dejaba sin plazo el drenaje posterior de un body HTTP incompleto.
  El test nuevo reprodujo FAIL y pasó al conservar deadlines durante flush/drain.
  La evidencia externa conserva ambas ejecuciones, sin ocultar la primera.
- **Validación final local:** Go 1.24.4, GOMAXPROCS=2, GOTOOLCHAIN=local,
  GOFLAGS=-p=1, GOPROXY=off. 1042 PASS (162 tests principales + 880 subtests),
  0 FAIL, 0 SKIP; cobertura de sentencias 88,1 %. gofmt, go vet -p 1 ./...,
  make check (incluye race ./..., build, 25 Python, Nova, Terraform/Ansible y
  documentación/evidencias), go mod verify y build explícito terminaron en 0.
  La cobertura no acredita integración real ni durabilidad.
- **Filesystem real temporal:** reapertura de componentes, .part/.ready/.blob,
  corrupción/ausencia, tombstones y limpieza idempotente. Test de flock entre
  procesos propios temporales. Recorrido POST/PUT/GET/PATCH/DELETE usa SQL doble
  y adaptador Keystone contra un fixture HTTP: no es Keystone real.
- **Preparado, NO EJECUTADO:** TestB25MySQLVersionedUpgrade,
  TestB25MySQLConcurrentFences y TestB25MySQLFilesystemHTTP. Compilación con tag
  integration y run vacío aprobada, sin ejecutar casos. Cada suite requiere su
  nuevo esquema independiente y autorización posterior; la última exige además
  runtime separado y root propio. Auth simulada se etiqueta; modo Keystone real
  exige credencial/token preexistentes autorizados y nunca hace fallback a fake.
- **Límites:** 000/001/002, store y código MySQL previo intactos. Sin VM,
  contenedores, servicios reales, DDL/DML de laboratorio, cuentas/grants,
  benchmarks, Nova o Git de escritura. Sin prueba de proceso terminado durante
  transferencia, reinicio MySQL B25 o pérdida de energía. No se declara B25
  completo. Binding no distingue generaciones recreadas del mismo esquema en
  el mismo server_uuid; requiere gestión humana conjunta SQL/store y primario
  directo. Guía reproducible: docs/glance/b25-operations.md.

## 2026-10-29 — B26, N2: autenticación compuesta acotada

- **Implementado:** validación independiente del token entrante de servicio y
  del usuario, preservando la identidad y roles originales del usuario. El
  servicio requiere rol service, proyecto igual al validador y vigencia normal;
  su rol admin no se delega. La credencial propia sigue sin admitir admin.
- **Contrato leído:** Nova fijado 247ea5d y keystonemiddleware del laboratorio;
  Keystone api/auth.py y token/provider.py confirman allow_expired=true con la
  ventana del servidor y comprobación de revocaciones. No hay ventana local
  codificada. Sólo tras validar el servicio se solicita esa validación de usuario.
- **Seguridad:** duplicados, vacíos y listas de tokens se rechazan; identidades
  cliente no se cachean. Restricciones bind/access_rules no implementadas fallan
  cerrado. Sin cambios de observabilidad, SQL, store, endpoints o servicios.
- **Validación dirigida:** 326 resultados PASS, incluidos 72 de los tests y
  subtests nuevos; go test -p 1 -count=1 -json ./internal/auth
  ./internal/glance/api y go vet -p 1 ./... terminaron en 0 con Go local,
  GOMAXPROCS=2 y GOPROXY=off. Evidencia y códigos de salida fuera del repositorio
  en b26-n2-composite-20261028a. Ver contrato en docs/glance/composite-auth.md.
- **Regresión general:** make check terminó en 0; incluye formato, documentación
  y evidencia histórica, 25 pruebas Python, pin Nova, sintaxis Terraform/Ansible,
  vet Go, tests de todos los paquetes con race detector y compilación. go mod
  verify terminó en 0. No se ejecutó ninguna suite opt-in de infraestructura.
- **Límites:** servidores Keystone de protocolo locales y repositorios dobles;
  no valida autenticación compuesta contra Keystone real, MySQL real, cliente
  Nova contra Go ni arranque de Ubuntu. N3–N7 no ejecutados en esta acción.

### 2026-10-29 — Revisión final de las aserciones HTTP N2

- Se sustituye la comparación de referencias compartidas por una instantánea
  JSON independiente de todos los valores de Image antes de la solicitud. El
  fixture incluye nombre no nulo, tags y propiedades para ejercitar aliasing.
- PATCH autorizado exige persistir el nombre y updated_at, conservar los demás
  metadatos y reflejar el cambio tanto en la respuesta como en un GET posterior.
  DELETE exige marcar el borrado, respuesta vacía, GET 404 y exclusión del listado.
- Cinco fallos inyectados sólo en copias externas del doble de repositorio
  (puntero, mapa, slice, PATCH sin efecto y DELETE sin efecto) fueron detectados
  por las aserciones esperadas; cada ejecución negativa terminó en 1. No se
  modificó el doble original ni código productivo para esas inyecciones.
- Pruebas dirigidas y dirigidas con race terminaron en 0, con paralelismo 1 y
  GOMAXPROCS=2. Logs y códigos de la revisión en b26-n2-final-review-20261029a,
  separados de la evidencia N2 previa. Sin cambios de semántica productiva,
  infraestructura, servicios ni avance a N3. No acredita integración real.

## 2026-10-29 — Resolución local de conflictos PR #5

- **Base:** merge iniciado por el usuario entre `feat/glance-b25` (`aa6e466`)
  y main de Paolo (`0279987`), con base común `d0d1b32`. No se inició ni se
  finalizó el merge en esta revisión.
- **Resolución:** README conserva el estado de Glance B25 y Placement B07.
  El loader compartido conserva los namespaces y defaults de ambos servicios;
  Glance mantiene `store-scope`/`store-init` y su política binaria, mientras
  Placement sólo acepta `serve`/`migrate` y no consulta variables `GLANCE_GO_*`.
  Se conservaron el código, pruebas, documentación y evidencia entrantes.
- **Regresiones:** `internal/config/namespace_test.go` comprueba independencia
  en ambos sentidos y rechazo temprano de comandos de store en Placement.
  El paquete config aprobó 82 resultados de tests/subtests (9 tests principales).
  Un overlay externo sin el aislamiento binario terminó en 1, como se esperaba;
  los archivos productivos no se modificaron para esa ejecución negativa.
- **Verificación:** `GOMAXPROCS=2 GOTOOLCHAIN=local GOFLAGS=-p=1 MAKEFLAGS=-j1
  make check` terminó en 0: documentación, evidencia histórica, pruebas Python,
  pin de Nova, formato/sintaxis de infraestructura, formato/vet Go, pruebas con
  race detector y compilación de ambos servicios. Sin integración MySQL o
  Keystone real ni operaciones sobre VM, contenedores o servicios.
- **Git:** el índice y sus entradas no se modifican; los dos conflictos siguen
  marcados `UU` hasta que el usuario revise y marque sus resoluciones.
  No se ejecutaron staging, commits, push ni finalización del merge.

## 2026-10-29 — B08: catálogos y asociaciones Placement

- **Base y alcance:** PR #6 integrado en b4d8300; rama feat/placement-b08.
  Reimplementación parcial para Nova: classes/traits estándar y CUSTOM,
  asociaciones por provider y cambios seleccionados 1.1–1.7 de sus rutas.
  Default 1.0, latest 1.7; no se afirma cobertura completa de la API.
  Nova fijado usa traits 1.6 y ensure resource classes 1.7 según report.py.
- **Implementación:** catálogo persistido (21 clases y 377 traits fijados a
  os-resource-classes 1.1.0 y os-traits 3.9.0 del laboratorio); migración v3
  append-only, v1/v2 intactas. CUSTOM CRUD, protección de estándar/en uso,
  renombre de clase con cascade sin cambiar generation, traits con CAS y
  no-op sin incremento, filtros asociados. Aggregates 1.1, member_of 1.3,
  resources 1.4 y DELETE inventories 1.5 cubren dependencias seleccionadas.
  API/servicio/repositorio separados, sin importar dominios Glance.
- **Pruebas previas y correcciones:** nombre CUSTOM y protocolos inicialmente
  sin implementar; tests antes de código. MySQL real detectó error 1826 al
  recrear una FK con igual nombre en un ALTER; v3 separa las sentencias.
  Cada reintento usa DB nueva; el historial dirty sigue bloqueado. La primera
  campaña dio 75/75 status y 0/75 estricto por Vary en snapshots 404 y textos
  de errores 404/405. Ocho respuestas Python capturadas se reprodujeron en
  tests rojos y se corrigió Go sin flexibilizar errores, tipos o generation.
- **Comparación viva final:** scripts/placement_catalog_differential.py,
  opt-in --allow-mutations, tokens privados y salida nueva: **82/82** status,
  respuestas y estados estrictos frente a Python f4a89d3 con MySQL/Keystone
  reales. Regresión del runner B07 sin modificarlo: **57/57** status, respuestas
  y estados en 1.0. Los recursos originales eran frescos (GET 404 previo) y
  quedaron ausentes tras limpiar. Se conservan también intentos 1 y 2.
- **Normalización explícita:** request_id/orígenes/Vary como B07; ordenar listas
  de traits/aggregates y catálogo de clases sin deduplicar. Sólo listados
  globales excluyen CUSTOM ajenos al ensayo; todos los estándar y propios se
  comparan, con respuestas crudas completas. Las asociaciones positivas de
  aggregates/member_of se verifican en MySQL aislado; en el original sólo PUT
  [] porque HTTP no permite borrar filas globales de aggregates. Esta parte
  no se atribuye a la comparación viva.
- **MySQL y proceso:** TestB08MySQLCatalogs con race en schema descartable,
  cuentas migrate/serve distintas: upgrade v2→v3 conserva inventario/generation,
  idempotencia, estándar/CUSTOM, CAS mixto traits/inventories, rollback tras
  overflow SQL después de modificar hijos y protección por FK. Pasaron además
  make placement-test-binary y make placement-test-upgrade en schemas nuevos
  separados. SIGTERM exit 0 y proceso nuevo conservan clase CUSTOM, inventario,
  trait y asociación con generation; no se reinició el servidor MySQL.
- **Gate local:** make check terminó en 0, con documentación/evidencia histórica,
  13 tests harness y 19 tests unitarios Python, pin Nova, sintaxis/fmt de
  infraestructura, formato/vet Go, pruebas de ambos módulos con race y build.
  La VM no se inicia desde este gate. Se verifica además que los 127 digests
  de fuentes/inputs guardados corresponden a los archivos ensayados.
- **Limpieza y límites:** proyecto/usuario/credenciales temporales Keystone
  eliminados, recursos de ambos Placement ausentes, contenedores/datos/procesos
  locales y secretos descartados; tt-openstack-lab apagada. No se cambió el
  endpoint original, Excel ni capturas S2. No se acredita Nova contra Go,
  allocations, candidates, benchmarks ni cierre transversal B11/B12. B09 debe
  restar usage al filtro resources antes de habilitar claims.
- **Evidencia y continuación:** evidence/sprint3/placement/b08-close-20261029/
  conserva reportes, errores iniciales, logs, restart, hashes y limpieza.
  docs/placement/b08-development.md documenta contrato/operación; PLANS.md y
  implementation-plan.md dejan B09 allocations como siguiente incremento.

## 2026-10-29 — B09-A: allocations atómicas y uso persistido

- **Base:** B08 6587805, PR #7 abierto al comenzar; rama feat/placement-b09.
  Entrega A implementada; **B09 sigue en progreso**. Incluye GET/PUT/DELETE
  allocations por consumer, GET allocations/usages por provider, reemplazo
  atómico y capacidad restante. Default 1.0, máximo seleccionado 1.8; falta
  formato por provider 1.12 y consumer generation 1.28 que exige Nova (PUT
  observado en 1.36). No se acredita integración Nova ni candidates.
- **Contrato y prueba inicial:** se inspeccionó código fijado f4a89d3 de handlers,
  schemas, objetos, políticas y excepciones. TestB09AllocationProtocols falló
  inicialmente con 404 para consumer vacío; luego pasa con 200 y allocations {}.
  Se mantienen diferencias entre GET inválido/vacío, DELETE repetido 404,
  array legacy, duplicado de provider que conserva la última entrada y roles.
  Nueve errores Python capturados se reproducen literalmente como regresión.
- **Microversión 1.8 justificada:** la DB original no tenía registros globales
  incomplete-project/user. Una escritura legacy los crearía sin limpieza HTTP.
  Se añadió el cambio puntual 1.8 (project_id/user_id obligatorios y persistidos
  junto a las reservas) y se usaron IDs preexistentes en Placement para la
  campaña. GET aún no muestra propietario; ese cambio pertenece a 1.12.
  Las escrituras 1.0–1.7 tienen pruebas locales, no comparación viva positiva.
- **Persistencia:** migración v4 append-only, historial v1–v3 y hashes intactos;
  serve rechaza v3/dirty. Consumers y allocations propios, FK a inventories,
  cascade para renombre de clase CUSTOM y GC de consumer al liberar. La fixture
  de upgrade B08 siembra v2 mediante SQL propio antes de migrar, evitando usar
  el repositorio actual contra un schema histórico sin allocations.
- **Transacciones:** lock de consumer y unión ordenada de providers anteriores/
  nuevos; READ COMMITTED permite leer uso actual tras ganar un lock bloqueado.
  Se descuentan las reservas anteriores del consumer. Fallar en cualquier clase,
  provider o actualización SQL revierte reservas, generaciones y propietario.
  Un PUT exitoso aumenta cada provider solicitado una vez; old-only y DELETE
  no lo aumentan, como el original. La comparación de capacidad evita redondear
  used a float (caso 2^53+1) y conserva límites fraccionarios/min/max/step.
- **Corrección al plan:** el original permite reducir inventario por debajo de
  uso y conserva la reserva en sobrecapacidad. Go reproduce ese comportamiento:
  no permite empeorar el exceso mediante una nueva reserva, pero admite igualar
  o reducirlo cumpliendo constraints. Se bloquean borrados de provider/inventario
  en uso. El filtro resources ahora resta allocations persistidas.
- **MySQL real:** tres incrementos de suites en schemas nuevos; la ejecución
  final TestB09MySQLAllocations con race prueba upgrade v3→v4 preservando B08,
  rollback por overflow final de provider y consumer, propietario sin cambios
  parciales, carrera entre dos consumers, carrera de varios providers, reemplazo
  concurrente del mismo consumer, pool nuevo y renombre de clase asignada.
  También PASS TestB08MySQLCatalogs, TestB07MySQLHTTP, TestBinaryMySQLLifecycle
  y TestV1CollisionStopsUpgrade, cada una en su propio schema vacío y con cuentas
  migrate/serve diferentes. MySQL 8.0.46 descartable en tmpfs, sin DB original.
- **HTTP real:** scripts/placement_allocation_differential.py con opt-in, tokens
  privados y directorio nuevo: primera campaña **52/52**, ampliación final
  **60/60** status/respuestas/estados estrictos; regresiones B07 **57/57** y
  B08 **82/82** con sus runners intactos. Keystone/MySQL reales; sólo recursos
  propios nuevos. Normalización B07 exclusivamente (request_id, origen Location,
  tokens Vary); no se cambian errores, generation, tipos o identidades.
- **Reinicio:** SIGTERM exit 0 y nuevo proceso conserva provider, inventario,
  allocations, usage y generaciones; DELETE y limpieza comprobados. No se
  reinició el servidor MySQL ni se simuló una caída abrupta.
- **Gate local:** make check terminó en 0: formato/vet Go, pruebas de ambos
  dominios con race y build, 13 tests harness y 21 unitarios Python, documentos/
  evidencia histórica, pin Nova y sintaxis/fmt de infraestructura. No inicia VM.
- **Limpieza:** lectura SQL posterior del original confirma 0 providers, 0
  consumers y 0 allocations del ensayo, mismos registros globales project/user
  y ausencia de nuevas filas incomplete. No hubo escrituras SQL directas al
  original. Identidad/credenciales de ensayo eliminadas, procesos/contenedores/
  datos/archivos privados locales descartados y VM apagada. Sin cambios de
  endpoints Nova, Excel ni evidencia S2.
- **Evidencia y siguiente:** evidence/sprint3/placement/b09a-20261029/ conserva
  60 casos, regresiones, ensayos previos, rollback/concurrencia, hashes, restart
  y limpieza. docs/placement/b09-development.md documenta operación y B09-B:
  formatos modernos, consumer generation y dependencias antes de anunciar más
  versiones. El rollback binario a B08 requiere restaurar su DB propia previa;
  no se implementa down migration automática.

## 2026-10-29 — Cierre técnico B09 y continuación autorizada hasta B12

- El usuario pidió terminar B09 y amplió el trabajo hasta B12. Se conserva el
  subconjunto Nova; no se acredita sustitución ni boot de B13–B15.
- B09-B añade formato por provider 1.12, owner en GET, consumer generation 1.28
  distinguiendo ausente/null/zero/obsoleta y mappings 1.34. PUT 1.36 y lecturas
  modernas forman el perfil priorizado. POST batch y reshaper quedan fuera.
- CAS del consumer dentro de la misma transacción que reservas y propietario;
  un conflicto no tiene efectos. PUT vacío libera y elimina el consumer, con
  incremento de los providers anteriores; DELETE sigue sin incrementarlos.
  El provider generation opcional del payload se ignora como en upstream.
- Migración v5 append-only añade created_at a allocations. Se conservan v1–v4
  y hashes; Last-Modified proviene de reservas persistidas y Cache-Control es
  no-cache desde 1.15. Desde 1.23 los errores incluyen code; conflictos de
  consumer usan placement.concurrent_update.
- Comparación real con MySQL/Keystone: **24/24** casos modernos estrictos,
  regresiones B09-A **60/60**, B07 **57/57**, B08 **82/82**, con cleanup.
  Se conserva el valor crudo de Last-Modified, se valida HTTP date y sólo se
  normaliza el timestamp además de request_id/orígenes/Vary permitidos. Sin
  normalizar códigos, status, generación, owner o tipos.
- Cinco suites MySQL independientes PASS: B09 añade carrera de creación del
  mismo consumer, CAS stale/null, owner inmutable y liberación vacía; también
  pasan B08/B07, lifecycle binario y guard de colisión. make check PASS con
  race/vet/build de ambos módulos, documentos, Python e infraestructura.
- El máximo 1.36 se aplica a allocations; otras rutas mantienen 406 explícito
  fuera de su perfil 1.8 hasta B10. No se afirma compatibilidad íntegra 1.36.
- Evidencia en evidence/sprint3/placement/b09-close-20261029/, con hashes, raw
  HTTP, SQL, restart y limpieza. SQL original sólo lectura; metadata global no
  cambió. Identidad temporal eliminada y VM apagada antes de seguir trabajo
  local B10. No se modificaron Nova/endpoints, Excel ni capturas S2.

## 2026-10-29 — Cierre técnico B10 del perfil plano

- Base cierre B09 0e59cf3; rama feat/placement-b10. Candidates procede de una
  instantánea RR de MySQL (providers, inventory, usage, traits y aggregates).
  Evalúa capacidad remanente, min/max/step, traits requeridos/prohibidos,
  root_required, limit, pertenencia/negación de aggregate, grupos/policy,
  in_tree y same_subtree para providers planos. No hay respuestas constantes.
- Representaciones 1.10/1.12/1.17/1.27/1.29/1.34; máximo 1.36 en candidates.
  No se promete toda API ni árboles/sharing. Parent no nulo y estado compartido
  se rechazan explícitamente; dos grupos isolate no comparten un root plano.
- Perfiles Nova contrastados: providers 1.14/1.20 (root/parent, links, required,
  in_tree y POST 200), aggregates 1.19 con CAS y aumento aun en no-op, inventory
  1.26 con capacidad cero/reserved=total. Cada ruta tiene máximo explícito y
  devuelve 406 fuera del perfil. Migración v6 conserva v1–v5 y persiste tiempos
  de provider/inventory; timestamps históricos desconocidos son de migración.
- Test inicial de candidates falló con 406; ahora devuelve 200 y resultado vacío
  al no existir inventario. Suite SQL real prueba uso/traits/aggregate, CAS de
  aggregate y 40 lecturas concurrentes con relación generation/usage que detecta
  mezclas de snapshots. Las cinco suites previas también pasan en DB nuevas.
- Comparación viva MySQL/Keystone: **44/44** estricto, ocho seeds sin diferencias.
  Regresiones B09 moderno **24/24**, B09-A **60/60**, B07 **57/57**, B08 **82/82**.
  El CUSTOM resource fresco excluye providers originales ajenos a la prueba.
  Last-Modified validado y normalizado como timestamp; se ordenan conjuntos sin
  deduplicar; no se cambian códigos, generaciones, tipos ni capacity/used.
- make check PASS: vet/race/build ambos módulos, Python, documentos e infra.
  Sólo [] en aggregates original para no dejar filas globales; asociación
  positiva se verifica en MySQL propio. Cleanup HTTP y SQL sólo de lectura
  confirman ausencia de filas propias y metadatos globales intactos.
- Evidencia: evidence/sprint3/placement/b10-close-20261029/. Identidad/VM quedan
  temporalmente en uso por B11/B12; su limpieza final se registrará al terminar.
  No se cambió Nova, endpoints, Excel ni S2; B13–B15 conservan boot/sustitución.
