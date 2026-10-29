# Revisión del PR #5 — Glance B25 y autenticación compuesta

Fecha: 2026-10-29 (America/Santiago).

- [PR #5](https://github.com/PaoloCastanon/modules-devstack-optimized/pull/5).
- Base: `027998759c338bd81d9232427ea135dffc3f5bc0`.
- HEAD revisado: `fbb4a3d24bdd8b102e2e24c1a7b052e1563a41b5`.
- Resultado: **APPROVED**, sin defectos bloqueantes detectados.
- [Aprobación ligada al SHA](https://github.com/PaoloCastanon/modules-devstack-optimized/pull/5#pullrequestreview-5698455549).
- Merge autorizado por el usuario y confirmado: `4c07cb5fcc1b023b3f607ffc9f3788f40176f41f`.

## Alcance y confianza

75 archivos, 13004 líneas añadidas y 142 eliminadas en 13 commits. Revisión
focalizada por riesgo: código productivo nuevo/modificado, dependencias
inmediatas, historia de controles reemplazados y pruebas relevantes. Auth
compartida, filesystem y transacciones tienen riesgo alto. No se hizo auditoría
exhaustiva línea a línea de pruebas y documentación. Confianza moderada:
MySQL/filesystem reales ejecutados independientemente; Keystone/Nova reales y
fallos físicos fuera de esta revisión.

| Resultado | Cantidad |
|---|---:|
| Vulnerabilidades críticas/altas identificadas | 0 |
| Defectos bloqueantes identificados | 0 |
| Seguimientos de evidencia/cobertura | 2 |

La ausencia de hallazgos no demuestra ausencia de vulnerabilidades.

## Historia y controles

El rango comienza con `9346f05` (contrato B25), añade store, operaciones SQL,
coordinación, HTTP y arranque; `aa6e466` incorpora auth compuesta y `fbb4a3d`
integra Placement/main. Dependencias Go, workflow, Excel y capturas S2 no cambian.
Las migraciones 000/001 conservan su contenido.

git blame atribuye a `46b4222` el rechazo anterior de X-Service-Token en
api.go:99–104 de la base. El reemplazo exige validación compuesta explícita y
conserva Identity.Check; un autenticador sin esa capacidad falla cerrado. No se
identificó eliminación de una corrección de seguridad histórica.

| Área | Controles contrastados |
|---|---|
| Auth | Tokens de usuario/servicio independientes; servicio vigente con rol service y proyecto del validador; allow_expired sólo después de validar servicio; Keystone decide ventana/revocación; roles/proyecto originales del usuario; sin caché de clientes; restricciones access_rules/bind no soportadas rechazadas |
| Configuración | Namespaces Glance/Placement separados; store sólo en Glance; credenciales privadas; límites finitos; verificación y recuperación antes de listener |
| HTTP | Auth anterior al router, límites reales del stream/tamaño, deadlines, rangos simples, cierre de Body y aborto de descarga truncada; logs sin secretos |
| Filesystem Linux | Raíz privada inicializada explícitamente, scope SQL, flock exclusivo, descriptores fijados, O_NOFOLLOW, UUID canónico, publicación sin sobrescribir, hash/longitud y fsync, borrado de intento exacto |
| Coordinación | Capacidad acotada, exclusión upload/delete, relectura del fence antes de I/O, resolución de commit incierto, tombstone antes de eliminar bytes, recuperación sin promoción de estados incompletos, drenaje antes de liberar store |
| SQL | Parámetros, locks imagen→operación, CAS por intento/fase, recibo coherente con active, v2 append-only, checksum/dirty y verificación estructural |

## Escenarios adversariales y radio de impacto

Se siguieron: reader con servicio admin intentando escribir; servicio vencido,
revocado o de otro proyecto; usuario vencido bajo protocolo compuesto; acceso a
private/shared ajena; payload mayor que el límite/declaración; symlink en
raíz/entrada; intento obsoleto publicando/borrando; DELETE contra upload activo;
commit SQL incierto; interrupción entre publicación y activación; descarga
abierta durante DELETE; shutdown con I/O activo. Controles y regresiones están
presentes. Revocación/expiración compuesta se ejecutan con servidor de protocolo
de prueba, no con Keystone desplegado.

Conteo estático de sitios de llamada productivos directos, excluidos tests:

| Función | Sitios | Efecto |
|---|---:|---|
| Authenticate por interfaz | 2 | Entrada HTTP Glance/Placement |
| AuthenticateComposite por interfaz | 1 | Entrada HTTP Glance |
| Identity.Check | 15 | Auth, handlers y servicios de ambos dominios |
| Upload / OpenDownload / DeleteImage | 1 cada una | Binarios y DELETE Glance |
| RecoverUploads | 1 | Arranque exclusivo Glance |

El impacto transitivo de auth alcanza todas las rutas autenticadas de ambos
servicios. Placement conserva validación de un solo token y política propia;
las pruebas de namespaces verifican que no incorpora configuración binaria.

## Verificación independiente

HEAD exportado con git archive a `/tmp/trabajotitulo-pr5-GWYVV7`. Cada suite
MySQL recibió esquema nuevo independiente y cuentas dedicadas en un contenedor
MySQL 8.0.46 con tmpfs y publicación sólo loopback. La suite HTTP utilizó cuentas
migrador/runtime separadas y raíz privada nueva. Al terminar se retiraron
contenedor, datos y secretos temporales.

| Comando/evidencia | Resultado |
|---|---|
| make check sobre HEAD | PASS: harness, 16 tests Python unit, pin Nova, evidencia, ShellCheck, Terraform/Ansible, formato Go, vet, race ./... y build |
| go test -coverprofile=/tmp/pr5-coverage.out ./... | PASS; 79,6% global, incluyendo Placement |
| go test -race -count=1 -tags=integration ./internal/glance/repository/mysql -run '^TestB25MySQLVersionedUpgrade$' -v | PASS: upgrade v1→v2, conservación, idempotencia, historia alterada/dirty rechazada |
| go test -race -count=1 -tags=integration ./internal/glance/repository/mysql -run '^TestB25MySQLConcurrentFences$' -v | PASS: serialización/CAS y conflictos entre operaciones |
| go test -race -count=1 -tags=integration ./cmd/glance -run '^TestB25MySQLFilesystemHTTP$' -v | PASS: HTTP→MySQL→filesystem, upload/download, PATCH, reapertura de pool/owner/server, DELETE y limpieza; autenticación doble |
| git diff --check origin/main...origin/pr-5 antes del merge | PASS |
| [CI 40876937535](https://github.com/PaoloCastanon/modules-devstack-optimized/actions/runs/40876937535) | local-checks completed/success en el SHA revisado |
| virsh -c qemu:///system domstate tt-openstack-lab | shut off |

Cobertura sin tags: auth 91,1%; config 94,1%; Glance API 96,3%; modelo 87,4%;
MySQL 85,4%; servicio 90,3%; store 91,4%; cmd/glance 56,6%. No incluye las tres
ejecuciones reales posteriores. Reabrir el pipeline no reinicia el proceso ni
el servidor MySQL. El árbol del merge coincide exactamente con el HEAD revisado.
make check también aprobó en main integrado con este informe y el build log;
git diff --check aprobó los cambios documentales de revisión.

## Seguimientos y límites

1. **Trazabilidad real.** El cuerpo del PR declara CLI completo, descarga de
   621284352 bytes y boot Ubuntu/Nova ACTIVE con configuración restaurada. El
   diff aún documenta la integración real como pendiente y no incorpora los
   artefactos N3–N7 de esas afirmaciones. Enlazar evidencia saneada y actualizar
   estado operativo. Esta revisión las identifica como resultados del autor;
   no las reejecutó ni las acredita como evidencia independiente.
2. **CLI y fallos externos.** main/run tienen 0% en la suite sin tags, aunque
   startup/HTTP se prueban y el pipeline usa sus componentes reales. Quedan
   ejecución CLI de Glance como proceso en entorno independiente,
   expiración/revocación compuestas en Keystone real, corte de proceso durante
   transferencia, reinicio MySQL y pérdida de energía. Son límites de
   validación, no defectos demostrados.

El binding no distingue recreación del mismo esquema/servidor: requiere raíz
nueva y conservar la pareja SQL/store al restaurar, según el contrato del PR.
No se acredita compatibilidad íntegra Glance v2, durabilidad física, benchmarks
ni cierre formal de backlog. Skill aplicada: differential-review. VM apagada.
