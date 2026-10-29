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
