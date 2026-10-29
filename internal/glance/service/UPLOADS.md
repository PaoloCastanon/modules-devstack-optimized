# B25 — coordinador de upload y recuperación (Acción 5/8)

La descripción siguiente conserva el alcance de Acción 5. La extensión posterior
de DELETE y recuperación terminal de Acción 7 se documenta en
[b25-deletion.md](../../../docs/glance/b25-deletion.md): ahora RecoverUploads puede
terminar cleanup/deleted y cleanup/killed confirmados. DELETE rechaza writers
vivos con 503 y comparte admisión/ownership; no se abandona un writer ni se limpia
un active no eliminado. Download se añadió en Acción 6.

`UploadCoordinator` coordina ImageReader, repository.Transfers y store.Store.
No añade HTTP, download, DELETE de imágenes ni un reconciliador de tombstones.
MySQL 8/002 y la coordinación completa con bytes reales siguen sin validación
integrada. Las suites de fallos usan dobles independientes SQL/store.

## Construcción y admisión

En Linux, `NewUploadCoordinator` recibe un Filesystem YA abierto mediante
store.NewFilesystem, con flock exclusivo. Rechaza un segundo coordinador para el
mismo objeto. Debe ser el único propietario de sus operaciones: el llamante no
puede escribir/cerrar el store ni mutar intenciones en paralelo por fuera de este
protocolo. El UID/root y el código que configura dependencias son de confianza.
Inicialización del root, vínculo de scope con servidor/esquema SQL y configuración
de conexiones al primario siguen siendo responsabilidades explícitas del arranque;
ningún valor de una petición establece esas dependencias.

El constructor no inicia servicios y deja uploads bloqueados. RecoverUploads
puede ejecutarse sólo en arranque antes de admitir tráfico y sin writers activos.
Sólo el éxito completo habilita Upload. No hay una opción pública para saltar esta
recuperación, ni un booleano de petición que afirme exclusividad. El seam genérico
para store/SQL falsos es privado y se usa únicamente en las pruebas del paquete.
Close rechaza writers/recuperación activos; no cierra las dependencias. Al cerrar,
libera la pertenencia de coordinador, no el flock propiedad del Filesystem.

Límites obligatorios de servidor: MaxBytes >0 y <=1TiB (techo, no valor por defecto),
MaxConcurrent 1..64, ResolutionTimeout >0 y <=1min, RecoveryTimeout >0 y <=10min.
Se rechaza capacidad ocupada, sin crear una cola de readers bloqueados. No se
reserva RAM por Content-Length; el store usa buffers acotados. Se permite longitud
desconocida y se compara una longitud declarada no negativa con el tamaño medido.
La admisión de un objeto vacío conserva la regla del componente store; equivalencia
HTTP con Python aún NV. src es del llamante: debe proporcionar deadlines, Body.Close
o un Reader cooperativo para cancelar un Read bloqueado; no hay goroutines que
abandonen una lectura. Un fsync del kernel tampoco es interrumpible universalmente.

## Flujo y autorización

Upload recibe identidad ya validada por Keystone, UUID de imagen, Reader y longitud
opcional. Vuelve a comprobar estructura, expiración, scope y roles; usa writePolicy
B20 (member propietario/admin, 404 private/shared ajena, 403 public/community ajena).
No autentica tokens por sí mismo ni reemplaza Keystone. Protected no impide upload.
No acepta attempt_id, root, límite de servidor o destino de limpieza del cliente.

Después de autorizar/validar, verifica estado SQL, genera UUIDv4, reserva y confirma
reserved/saving por DataState. Relee catálogo/formato tras la reserva: PATCH podía
cambiar formatos mientras queued; en saving ya no puede. Un formato ausente o
inválido en esa relectura provoca compensación segura antes de escribir bytes.

Stage -> StageData -> Publish -> PublishData -> CommitData. Los checkpoints exigen
identificador/fase y recibo exactos; ningún lock SQL queda abierto durante I/O.
La finalización comprueba active/committed y todos los campos calculados. Raw usa
virtual_size=size. QCOW2 y otros formatos sin inspector verificado conservan null;
no se inspeccionan backing files ni se inventa un tamaño virtual. El inspector
QCOW2 sigue pendiente como límite explícito, no se afirma equivalencia de formato.

Después de Publish se revalida el fence y se abre/cierra el blob para contrastar
su tamaño y MD5/SHA-512 con el recibo. Si Publish devuelve error, incluso con blob
íntegro, no se considera confirmado el fsync: no se promueve. Si no puede verificarse
SQL o integridad, se retiene la evidencia. Con estado seguro y blob ausente o
íntegro, se confirma cleanup antes de borrar. Stage ErrExists conserva la colisión
para reconciliación, no sobrescribe ni elimina automáticamente.

Un commit incierto se resuelve con DataState nuevo, al primario, con contexto propio
acotado. Sólo el estado exacto esperado permite continuar. Si no se aplicó, el upload
no se reintenta automáticamente; se compensa su intención si puede confirmarse.
Si no se puede resolver, se devuelve ErrUploadReconciliation sin I/O destructivo.
Errores públicos no incluyen diagnósticos crudos del Reader, SQL, rutas o DSN.

Un commit final confirmado puede ser éxito interno aunque el cliente cancele o
no reciba la respuesta. Un tombstone posterior con el mismo intento y recibos
persistidos prueba la finalización anterior: no se devuelve Gone retrospectivo ni
se restaura/borran sus bytes. Si DELETE ganó antes del commit, se devuelve
ErrUploadGone y se delega su limpieza. Esto no implementa DELETE ni sustituye la
exclusión de publicación y cancelación del writer que tendrá que integrar Acción 7.

## Compensación y recuperación

El writer termina Stage/Publish antes de compensar. Sólo un intento propio saving
sin datos calculados puede registrar cleanup a queued; se relee esa intención,
se ejecuta Delete de la clave exacta y se confirma cleaned/queued después de que
Delete haya terminado con éxito, incluidas las sincronizaciones del store.
Relecturas detectan cambio de intento/tombstone antes y después de eliminar.
No se limpia active, ni se convierte un fallo genérico en killed.

Un error de Delete, incluso después de unlink, mantiene cleanup/saving y bloquea
la nueva reserva. Un error SQL al finalizar obliga a releer; si no está confirmado
cleaned, se conserva la intención para reintento al siguiente arranque. Los contextos
de resolución/limpieza sobreviven a la cancelación de petición durante un máximo
ResolutionTimeout. La recuperación respeta su límite total entre candidatos; una
compensación segura ya iniciada puede añadir como máximo ese plazo al cierre.

ListDataOperations sólo proporciona candidatos keyset. Cada uno se relee de forma
autoritativa. En arranque exclusivo, reserved/staged/published y cleanup a queued
se limpian/restauran, nunca se promueven. Active/committed se conserva y verifica
contra el blob; una ausencia/corrupción bloquea admisión sin mutaciones. Cleaned a
queued se conserva. Cambios de intento, contradicciones, tombstones y limpieza
terminal se retienen y bloquean admisión: requieren reconciliación posterior,
incluida Acción 7. No se usa TTL ni se barre/elimina contenido desconocido.

## Evidencia y límites

Las pruebas comprueban orden de llamadas, fases permitidas, applied/not-applied,
fallos SQL/store y cancelación. Un test Linux usa un root temporal real sólo para
comprobar exclusividad de construcción; no demuestra persistencia integrada.
La regresión del catálogo B20 forma parte de make check. MySQL real de 002,
pérdida de energía, fsync físico, reinicio del proceso durante transferencia,
Keystone/HTTP binario y coordinación DELETE no están verificados en esta acción.
