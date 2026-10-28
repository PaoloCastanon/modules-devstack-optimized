# B25 — persistencia de intenciones (Acción 4/8)

Migración append-only 002: `image_data_operations`, una fila vigente por imagen,
UUID de intento único, FK sin cascada, InnoDB, CHECK de fases/tamaños/recibos/destinos
 y códigos de error acotados. Índice `(phase, updated_at, image_id)` para selección
futura de reconciliación. No modifica 000/001. Migrate registra dirty antes del DDL,
inspecciona la estructura nueva antes de confirmar la versión 2 y relee el historial.
VerifySchema exige 1 y 2 y comprueba tabla/columnas/índices/FK y CHECK habilitados.
No interpreta equivalencia lógica de expresiones CHECK preexistentes modificadas
fuera del migrador: el checksum identifica el SQL aplicado, no un hash del DDL vivo.

`repository.Transfers` es independiente del catálogo B20 y no importa el store.
No realiza Stage/Publish/Open/Delete ni conserva transacciones durante I/O.
La autorización y orquestación HTTP/servicio siguen pendientes de Acciones 5/7.
Estas operaciones internas no deben exponerse directamente al cliente.

- `ReserveData`: UUID fresco generado por el servidor llamante, estado queued sin
  campos calculados, sin intención o con intención cleaned a queued. Bloquea la
  imagen antes de la intención. Cambia saving y reserved en una transacción.
  El llamante no puede reciclar UUID de intentos anteriores; no es un journal
  histórico de todos los intentos. Un error no autoriza escribir archivos.
- `StageData`: receipt MD5/SHA-512 hexadecimal minúsculo, tamaño medido y virtual
  opcional, comprobación de expected_size. Exige mismo intento reserved/saving.
- `PublishData`: staged a published. El repositorio confía en la confirmación de
  publicación que deberá aportar el servicio; no prueba existencia de bytes.
- `CommitData`: published/saving a committed/active. Sólo actualiza estado,
  tamaño, virtual_size, checksum, os_hash_algo=sha512 y os_hash_value; conserva
  nombre, formatos, tags, propiedades y otros campos editables de PATCH.
- `BeginDataCleanup`: invalida finalización tardía. queued/killed sólo desde
  saving no committed; deleted requiere tombstone previamente confirmado. No
  implementa DELETE ni sustituye la futura transacción conjunta de tombstone e
  intención de limpieza. El destino sólo puede cambiar para respetar deleted.
- `CompleteDataCleanup`: exige mismo intento cleanup. Sólo restaura queued/killed
  desde saving no deleted. Nunca restaura active/tombstones; deleted se conserva.
  El servicio debe confirmar primero la eliminación y fsync del filesystem.
- `DataState`: transacción nueva con bloqueos, incluye tombstone e intención.
  Debe consultarse al primario autoritativo para resolver un commit incierto.
- `ListDataOperations`: inventario keyset por image_id, máximo 1000 por página.
  Es una lista de candidatos; releer DataState antes de actuar. No usar timestamp
  como prueba de abandono ni limpiar un escritor vivo.

`ErrCommitUncertain` significa que el commit pudo aplicarse: no afirmar rollback,
activar, compensar o eliminar archivos sin relectura. La reserva devuelve la clave
propuesta incluso con commit incierto. Ante error de lectura posterior, conservar
archivos/intenciones y reportar dependencia no disponible. El repo no reintenta
mutaciones automáticamente. Los tiempos son UTC con microsegundos monótonos.

## Validación

Las pruebas normales usan dobles SQL. El doble concurrente modela bloqueo y
visibilidad de dos reservas; no acredita scheduling InnoDB ni durabilidad física.
Las pruebas de esquema comprueban metadatos simulados, no ejecutan DDL real.

`TestMySQLTransferOperations`, build tag `integration`, está preparado y NO
EJECUTADO en esta acción. Exige `GLANCE_GO_MYSQL_TEST=1`, archivo privado
`GLANCE_GO_TEST_DSN_FILE`, ValidateTestDSN y un esquema nuevo vacío independiente.
Cuenta dedicada con CRUD/CREATE/REFERENCES sólo en ese esquema; CheckAccount
conserva sus guardas. No reutilizar esquemas A/B ni el de pruebas de DDL parcial.
La prueba ejecuta migraciones y modifica filas: requiere autorización posterior
específica. Deja los datos para revisión, no elimina esquemas/cuentas/volúmenes.
No confirma Keystone, bytes, reinicios reales o durabilidad ante pérdida de energía.
