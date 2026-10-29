# B25 — DELETE coordinado y recuperación (Acción 7/8)

Actualización Acción 8: [arranque y cierre operativo](b25-operations.md) conectan
este coordinador al CLI. El alcance pendiente descrito al final es el de Acción 7;
las pruebas reales SQL/bytes y Keystone B25 continúan pendientes por separado.

Amplía DELETE `/v2/images/{id}` cuando el handler recibe
`WithBinaryTransfers(UploadCoordinator)`. Conserva el middleware Keystone B20,
la política member propietario/admin y protected incluso para admin. Sin esa
opción permanece el DELETE B20, limitado a queued sin contenido. Si se habilitan
bytes pero falta capacidad de eliminación coordinada, se devuelve 503: nunca se
usa el DELETE de metadatos como fallback. No existe DELETE `/file`.

## Exclusión y orden

El coordinador conserva la propiedad exclusiva del store y exige recuperación
completa antes de admitir PUT, GET o DELETE. Un indicador por imagen excluye la
vida completa del upload —incluida compensación y resolución SQL— de DELETE.
Otro indicador admite sólo un DELETE por imagen y bloquea nuevas reservas hasta
que termine. Son admisiones cortas bajo mutex: no se mantiene ese mutex ni una
transacción SQL durante I/O. Close y recuperación rechazan actividad pendiente.
Uploads, lectores y eliminaciones comparten el límite de capacidad del servidor.

**Decisión de esta acción:** DELETE durante un writer vivo devuelve 503 sin
tombstone ni unlink; el cliente puede reintentar cuando haya terminado. La
solicitud de Acción 7 autoriza expresamente rechazar/diferir si no es seguro
detener al writer. Esto difiere de la cancelación del upload prevista en sección J
del diseño B25 y no reproduce esa carrera Python. No se intenta abandonar un Read
bloqueado o un fsync. Se mantiene el fencing SQL ante cualquier finalizador tardío.
Una imagen saving sin writer local puede eliminarse con el protocolo siguiente.

1. Validar identidad, rol, UUID, propietario/visibilidad y protected.
2. Reclamar exclusión; leer DataState autoritativo para obtener el intento vigente.
3. TombstoneData bloquea primero la imagen y luego la operación, repite política
   y protected bajo el lock, y compara el intento esperado (nil exige ausencia).
4. En una transacción confirma deleted/deleted_at y cleanup target=deleted.
   Conserva attempt_id, recibos, propiedades, tags y UUID; no elimina filas.
5. Releer el resultado en una transacción nueva, con plazo propio acotado aunque
   la petición se haya cancelado. Nunca confiar en el estado propuesto retornado.
6. Con intención exacta confirmada, Store.Delete elimina sólo part/ready/blob de
   esa clave y sincroniza directorios. La ausencia de nombres permite reintentar
   fsync; un error después de unlink también conserva cleanup y devuelve 503.
7. Releer el fence, ejecutar CompleteDataCleanup y comprobar cleaned/deleted
   desde SQL nuevo antes de devolver 204. No restaurar nunca un tombstone.

No hay atomicidad conjunta SQL/filesystem ni rollback de bytes mediante MySQL.
Un commit incierto que no pueda resolverse conserva evidencia y devuelve 503.
Un commit final aplicado sólo cuenta como éxito si se confirma cleaned después
de que Store.Delete haya devuelto éxito. El cliente puede no recibir ese éxito
si su conexión ya terminó. Un DELETE repetido devuelve 404 incluso si queda
cleanup pendiente: ese 404 no acredita limpieza física; la recuperación la termina.

## Transiciones seleccionadas

| Estado/intención actuales | Transacción de eliminación | Trabajo filesystem |
| --- | --- | --- |
| queued o killed sin operación ni campos calculados | deleted, sin crear intento | Ninguno |
| queued + cleaned/queued; killed + cleaned/killed | deleted + cleanup/deleted sobre el intento existente | Borrado exacto idempotente, luego cleaned |
| saving + reserved/staged/published; saving + cleanup/queued o killed | deleted + cleanup/deleted, mismo intento | Sólo sin writer; luego cleaned |
| active + committed con recibos públicos coincidentes | deleted + cleanup/deleted, conserva recibos | Borra blob confirmado, luego cleaned |
| deleted | 404 de la petición normal | Recuperación de la intención pendiente |
| Pares desconocidos/incompatibles | Conflicto, sin mutaciones | Ninguno |

Las mutaciones SQL son parametrizadas y comprueban RowsAffected=1. La cuenta
runtime conserva CRUD dentro del esquema propio; no necesita DDL o privilegios
nuevos. No cambian las migraciones 000/001/002 ni CheckAccount/ValidateTestDSN.
La autorización recibida por el repositorio es un callback de política puro.

## Recuperación

RecoverUploads sólo se ejecuta en arranque con flock exclusivo y sin writers,
readers ni DELETE admitidos. El inventario no acredita abandono: cada candidato
se relee con DataState y se comprueba image_id/attempt_id, fase, destino y recibos.

- deleted + cleanup/deleted: borrar/re-sincronizar la clave y confirmar cleaned.
- deleted + cleaned/deleted: conservar el tombstone sin nuevo I/O destructivo.
- saving + cleanup/killed: ejecutar la intención terminal explícita y terminar en
  killed/cleaned; no inferir ese destino de una excepción genérica.
- killed + cleaned/killed: conservar sin promover ni permitir upload.
- reserved/staged/published no eliminadas: conserva la compensación segura a
  queued de Acción 5; committed/active se verifica y nunca se limpia.

Tombstones con fase distinta de cleanup/cleaned, destinos incompatibles, recibos
contradictorios, intentos cambiados o SQL/store no disponibles bloquean admisión
y conservan evidencia. No se reparan automáticamente tablas ajenas ni se crean
intentos ficticios para huérfanos. No hay barridos del root ni criterio TTL.
Los reintentos tras fallar fsync vuelven a llamar Delete aunque los nombres ya
no existan. Las recuperaciones de cleaned son idempotentes entre nuevos dueños.

Un GET con descriptor ya abierto puede terminar después del unlink en Linux.
La eliminación no cierra ese fd; el reader sigue ocupando capacidad hasta Close.
204 confirma eliminación de nombres y sincronización, no borrado seguro de
sectores ni liberación de bloques antes de cerrar los descriptores restantes.
GET nuevo después del tombstone devuelve 404. Durante el preflight de una lectura
concurrente puede observarse la pérdida del objeto o el tombstone; nunca se
entregan bytes sin la comprobación SQL/store previa de Acción 6.

## HTTP y contraste Python

| Caso DELETE | HTTP seleccionado |
| --- | --- |
| Éxito SQL y limpieza confirmados | 204 sin cuerpo |
| Sin token / token inválido | 401 con WWW-Authenticate B20 |
| Reader, protected, public/community ajena | 403 |
| Private/shared ajena, inexistente o ya deleted | 404 |
| Estado o fence incompatible | 409 |
| Writer vivo, capacidad, recuperación pendiente, SQL/store o resultado incierto | 503 |

Se leyeron controlador/serializador
[images.py](https://github.com/openstack/glance/blob/a3e5f490b7a28b1161714c94acffe5308d24b567/glance/api/v2/images.py)
y el DELETE de
[dominio](https://github.com/openstack/glance/blob/a3e5f490b7a28b1161714c94acffe5308d24b567/glance/domain/__init__.py)
del commit fijado: éxito 204, Forbidden 403, NotFound 404 y rechazo de protected.
La fuente contempla timeout del store 503 y estados/errores adicionales que no
se generalizan aquí. No se ejecutó Python ni se equiparan todas sus políticas
configurables. El protocolo cleanup, 503 por writer vivo y JSON B20 son límites
explícitos del subconjunto Go; no se implementan delayed_delete ni multi-store.

## Validación y ejecución pendiente

Las regresiones usan dobles independientes SQL/store y prueban orden, fences,
applied/no-applied, fallos antes/después de unlink, recursos y concurrencia. Los
tests HTTP usan el coordinador y filesystem reales sólo en t.TempDir; SQL y
autenticación siguen simulados. No acreditan InnoDB ni pérdida de energía.

`TestMySQLCoordinatedDeletion`, tag integration, queda preparado y NO EJECUTADO.
Requiere GLANCE_GO_MYSQL_TEST=1 y GLANCE_GO_TEST_DSN_FILE privado, ValidateTestDSN,
CheckAccount y esquema NUEVO VACÍO independiente, con autorización posterior.
Ese caso ejecuta DDL/DML y conserva datos; no debe compartir esquema con las
otras suites. Comprueba SQL, relecturas y concurrencia, sin probar bytes o fsync.

El arranque cmd/glance, vínculo SQL/store, deadlines y validación real conjunta
siguen pendientes de Acción 8. Esta acción no inicia servicios ni cierra B25.
