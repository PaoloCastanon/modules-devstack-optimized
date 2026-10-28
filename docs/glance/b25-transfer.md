# B25 — Contrato seleccionado de transferencia de imágenes

Estado al 28/10/2026: **documentación del diseño aprobado en planificación**.
Esta acción no implementa ni valida B25. Las rutas binarias, el store y la
migración 002 descritos aquí son trabajo posterior. La integración de B20 no
demuestra su funcionamiento. No se promete toda Image API v2.18 ni integración
con Nova.

## Fuentes y grado de comprobación

- [Fundación B20](development.md) y [catálogo B20](b20-catalog.md).
- [Contrato Glance de Sprint 2](../sprint2/glance-api-scope.md),
  [ADR-02/03](../sprint2/design-decisions.md) y
  [comparación diferencial](../sprint2/compatibility-strategy.md).
- Capturas originales: [upload](../../evidence/sprint2/glance/image-upload.request.txt),
  [respuesta upload](../../evidence/sprint2/glance/image-upload.headers.txt),
  [imagen activa](../../evidence/sprint2/glance/image-active.response.json),
  [download](../../evidence/sprint2/glance/image-download.headers.txt),
  [integridad SHA-256](../../evidence/sprint2/glance/image-download.sha256.txt) y
  [DELETE](../../evidence/sprint2/glance/image-delete.status.txt).
- Referencia Python exacta: `openstack/glance`, commit
  `a3e5f490b7a28b1161714c94acffe5308d24b567`:
  [image_data.py](https://github.com/openstack/glance/blob/a3e5f490b7a28b1161714c94acffe5308d24b567/glance/api/v2/image_data.py),
  [images.py](https://github.com/openstack/glance/blob/a3e5f490b7a28b1161714c94acffe5308d24b567/glance/api/v2/images.py),
  [domain](https://github.com/openstack/glance/blob/a3e5f490b7a28b1161714c94acffe5308d24b567/glance/domain/__init__.py) y
  [wsgi.py](https://github.com/openstack/glance/blob/a3e5f490b7a28b1161714c94acffe5308d24b567/glance/common/wsgi.py).
- [API oficial Image v2](https://docs.openstack.org/api-ref/image/v2/).

Leyenda empleada en las tablas:

- **S2**: confirmado por las capturas archivadas, leídas en esta acción.
- **PY**: corrección verificada por el usuario y aportada al cierre de Fase 2.
  No equivale a una nueva lectura independiente del código Python en esta acción.
- **D**: diseño seleccionado para Go; todavía no implementado ni probado.
- **NV**: equivalencia o detalle del original NO VERIFICADO independientemente.

La consulta de red falló por DNS durante la planificación. Esta acción no exige
red ni recaptura datos del laboratorio. El listado exhaustivo de excepciones
Python, sus cuerpos de error y el status preciso ante un archivo perdido siguen
NV; no deben deducirse a partir de una captura de éxito. La tabla siguiente fija
los casos seleccionados sin generalizar a todas las excepciones upstream.

## A. Alcance y preservación de B20

B25 añade subida, persistencia filesystem, descarga y eliminación coordinada de
bytes a imágenes del catálogo B20. Conserva autenticación Keystone, propietario,
visibilidad, JSON de metadatos, listado, PATCH y tombstones/UUID reservados.
La base de metadatos y el store serán propios, aislados de OpenStack original.

No se modificarán migraciones 000/001, sus checksums ni capturas históricas.
Los resultados posteriores se registrarán aparte. El contrato no autoriza
aprovisionar infraestructura, alterar identidades o sustituir servicios.

## B. Endpoints y autorización

| Método y ruta | Precondiciones seleccionadas | Efecto de éxito |
| --- | --- | --- |
| PUT `/v2/images/{id}/file` | Identidad Keystone de proyecto; member propietario o admin; imagen queued; disk_format y container_format definidos; cuerpo octet-stream | 204 sin cuerpo; contenido confirmado y estado active |
| GET `/v2/images/{id}/file` | Reader o admin; acceso según propietario/visibilidad B20; autorizar antes de abrir archivos | 200 completo, 206 para rango único válido o 204 si no existe contenido asociado |
| DELETE `/v2/images/{id}` | Member propietario o admin; imagen no protegida | 204 después de confirmar tombstone y limpieza del intento/contenido |
| GET `/v2/images/{id}` | Reglas de lectura B20 | 200 con estado y campos calculados; eliminado da 404 |

No se crea `DELETE /v2/images/{id}/file`: la eliminación pertenece al recurso
imagen. Private/shared ajenas permanecen ocultas con 404; public/community ajenas
visibles no son modificables por su lector y devuelven 403. No se amplía la API
de membresías. `protected` impide DELETE, incluso al admin; no se utiliza como
prohibición adicional de upload/download. Estos permisos son la selección D
basada en B20; no equivalen a reproducir toda la política configurable Python.

## C. Matriz de respuestas HTTP

| Operación/caso | HTTP | Estado o efecto | Base |
| --- | --- | --- | --- |
| Upload correcto | 204 | queued → saving → active; tamaño y hashes confirmados | S2/D |
| Formatos ausentes | 400 | No publicar; permanece queued; Python produce ValueError al pasar a saving | PY |
| Tamaño declarado inválido o distinto del recibido | 400 | Sin activación; si se reservó, limpiar y restaurar queued | PY/D |
| Límite de imagen excedido | 413 | Rechazar antes de reservar si es conocido; si se descubre leyendo, limpiar/restaurar | PY/D |
| Almacenamiento lleno | 413 | ENOSPC/EDQUOT; limpiar y restaurar, sin convertirlo genéricamente en killed | PY/D |
| Media type incorrecto o ausente en PUT | 415 | Sin reserva ni escritura | D/NV |
| Upload repetido en active o estado no apto | 409 | No reemplazar el contenido | D/NV |
| Token ausente/inválido | 401 | Sin efectos; WWW-Authenticate de B20 | D, regresión B20 |
| Prohibición por rol/propietario | 403/404 | Mantener ocultación y reglas B20 | D |
| Imagen inexistente o eliminada antes de iniciar | 404 | Sin efectos | D, regresión B20 |
| Upload iniciado pierde frente a DELETE confirmado | 410 | Mantener deleted; no restaurar queued ni activar | PY/D |
| GET de contenido completo | 200 | Bytes idénticos, sin modificar metadatos | S2 |
| GET sin contenido/localización asociada | 204 | Sin cuerpo binario | PY |
| GET con un rango válido | 206 | Segmento exacto y Content-Range | D/NV para detalles |
| Range múltiple | 400 | Sin cuerpo binario; no ignorarlo ni producir multipart | PY |
| Range inválido o insatisfacible | 416 | Content-Range bytes */N cuando se conoce N | PY/D |
| Archivo confirmado perdido o corrupto | 503 | No confundirlo con ausencia de contenido; no fabricar bytes | D, divergencia |
| DELETE autorizado de queued/saving/active | 204 | Tombstone y limpieza confirmada | S2 para active; PY para carrera saving; D |
| DELETE de imagen protegida | 403 | Sin eliminación | D, regresión B20 |
| DELETE repetido | 404 | El trabajo persistente de limpieza no depende del reintento | D, regresión B20 |
| Limpieza pendiente impide una nueva subida | 503 | No reutilizar ni sobrescribir un intento pendiente | D, divergencia temporal |
| Dependencia no disponible o commit incierto no resuelto | 503 | Conservar intención; no ejecutar compensación destructiva a ciegas | D |

Autenticar y autorizar antes de revelar estado, tamaño o rangos. Una conexión
cerrada puede impedir entregar el status previsto: registrar por separado el
resultado interno y la respuesta efectivamente emitida. Después de iniciar un
cuerpo binario no se anexa JSON ni se promete cambiar el status enviado.

## D. Cabeceras, tamaños y transferencia parcial

PUT exige `Content-Type: application/octet-stream`. La ausencia de Content-Length
es válida: admitir transferencia chunked y contar los bytes reales. El framing
HTTP es responsabilidad de net/http; no reinterpretar Content-Length ni aceptar
cabeceras ambiguas para eludir su validación.

`x-openstack-image-size` es una declaración opcional. El diseño acepta un entero
decimal no negativo representable en int64, rechaza valores inválidos/repetidos
y compara la declaración con los bytes recibidos. Si también hay Content-Length,
ambos tamaños deben coincidir. La validación léxica exacta del original es NV.
La longitud incorrecta es 400; superar el límite o agotar almacenamiento es 413.
Nunca aceptar un prefijo truncado como una imagen correcta. La interrupción se
trata como fallo recuperable cuando no existe un commit final confirmado.

El límite binario se comprueba antes de recibir cuando se conoce la longitud y
durante streaming en todos los casos. Es independiente del límite JSON de B20.
La cabecera de tamaño no obliga a cargar el cuerpo ni a reservarlo en RAM.

GET usa `application/octet-stream` y conserva el `Content-MD5` hexadecimal de
la captura S2. `checksum` es MD5; `os_hash_algo` será `sha512` y `os_hash_value`
el SHA-512 hexadecimal del contenido completo. El SHA-256 de S2 es una prueba
externa de integridad, no un sustituto de esos campos. MD5 no autoriza accesos.

El diseño fija Content-Length para la respuesta completa o el segmento, sin
exigir que coincida el framing con el chunked observado detrás de Apache.
Un GET sin contenido da 204; un objeto confirmado de longitud cero, si es válido
según el contrato de upload, no debe confundirse con ausencia de contenido.
El tratamiento Python exacto del upload vacío queda NV y requiere un caso propio.

Se incluyen rangos únicos `bytes=inicio-fin`, `bytes=inicio-` y `bytes=-sufijo`.
Una respuesta parcial usa 206, Content-Range y longitud del segmento. Varios
rangos dan 400; un rango inválido/insatisfacible da 416. No usar un helper que
introduzca multipart o condicionales adicionales sin control. El valor exacto
de Content-MD5 para 206 sigue NV: la captura S2 confirma el GET completo; no se
debe inventar un checksum de segmento o afirmar equivalencia sin contrastarlo.

ETag e If-Range están expresamente fuera de este subconjunto; no se anuncian ni
se promete soporte condicional. No son requisitos de aceptación de B25.

## E. Estados de imagen y fases internas

```text
queued --reserva confirmada--> saving --publicación y commit--> active
                                |
                                +--error recuperable--> limpieza --> queued
                                |
                                +--causa terminal explícita--> killed

queued / saving / active / killed --DELETE autorizado--> deleted
```

`limpieza` es una fase persistente interna, no un nuevo estado Image API.
Ni un error HTTP 500/503 ni una excepción genérica implican killed. Cada causa
terminal deberá identificarse y probarse expresamente antes de implementarla;
el inventario exhaustivo de esas causas Python sigue NV. Los errores recuperables
400/413 seleccionados no se reclasifican como terminales por conveniencia.

No activar automáticamente tras un reinicio un intento cuyo commit no existe.
Si active/committed está confirmado, mantenerlo aunque el cliente no recibiera
204. No cambiar active a killed como efecto lateral de un GET que detecta un
archivo perdido. No permitir que el cliente escriba estados o hashes por PATCH.

## F. Restauración recuperable a queued

Antes de reservar, un rechazo deja queued sin cambios. Después de reservar:

1. Detener el escritor del intento y confirmar la intención de limpieza.
2. Eliminar solo su temporal/objeto y sincronizar los directorios afectados.
3. Confirmar juntos phase=cleaned y restauración a queued, con tamaño/hashes
   públicos anulables sin restos del intento fallido.
4. Permitir una nueva reserva solo después de esa confirmación.

La restauración exige mismo attempt_id, fase esperada, estado saving y ausencia
de tombstone. Si DELETE ganó, el upload responde 410 cuando todavía pueda hacerlo;
no ejecuta su restauración. Un callback antiguo tampoco modifica un intento nuevo.

Si falla la limpieza, conservar phase=cleanup con destino queued y bloquear una
nueva subida con 503. La restauración visible puede retrasarse respecto de Python:
esa divergencia deliberada evita queued operativo con objetos pendientes o una
sobrescritura insegura. No eliminar la evidencia persistente para aparentar éxito.

## G. Diferencias justificadas y límites de equivalencia

| Decisión Go todavía no implementada | Justificación y diferencia declarada |
| --- | --- |
| 503 si falta/corrompe un objeto confirmado | Distingue inconsistencia del store de la ausencia de contenido que da 204; el status Python preciso en cada punto del fallo no se revalidó aquí |
| Restaurar queued después de limpieza confirmada | Puede retrasar la restauración observada en Python si falla el filesystem; mantiene fencing y evita reutilizar restos |
| Recuperación conservadora sin promoción automática | Un intento incompleto se limpia/restaura; no se supone que Python tenga este protocolo propio |
| virtual_size null sin inspector seguro | No anunciar un tamaño virtual inventado para formatos distintos de raw |
| Rechazar backing externo al inspeccionar QCOW2 | Evitar leer archivos del host o rutas externas; regla de seguridad D, no equivalencia negativa demostrada |
| Permisos y 401 del subconjunto B20 | Conserva sus límites documentados, incluido cuerpo JSON 401 frente al texto capturado de Python |

Status, tipos JSON, estados y efectos no se normalizan para esconder diferencias.
Los detalles marcados NV necesitan contraste o una decisión explícita antes de
declarar compatible la implementación futura; la existencia de este contrato no
los convierte en verificaciones realizadas.

## H. Arquitectura prevista

API → Service → Repository/MySQL para metadatos e intenciones; Service → Store
para bytes. El handler no ejecuta SQL. El servicio recibe identidad verificada y
coordina autorización, estados y recuperación. El store ofrece staging por
streaming, publicación, apertura y eliminación con errores tipados.

Mantener las regresiones B20 y su serialización. La finalización de upload cambia
solo campos calculados/estado: no escribe una copia antigua del nombre, tags o
propiedades y no pierde un PATCH concurrente. Los formatos no cambian en saving.
El DELETE B25 ampliará la restricción B20 de queued sin contenido en una acción
posterior, conservando protected, tombstones, autorización y UUID reservado.

La configuración prevista separa plazos de metadatos, upload, download, SQL y
Keystone. El servidor actual tiene plazos de metadatos que no deben cortar archivos
grandes. Toda ruta tendrá deadlines efectivos y cancelación; no basta aumentar
un contexto si ReadTimeout/WriteTimeout siguen expirando antes. Conservar límites
JSON e introducir límites binarios, de concurrencia y capacidad independientes.

## I. Store, transferencia y publicación

Raíz propia, validada y separada del store Python; directorios 0700 y archivos
0600. Rutas derivadas solo de UUID de imagen/intento; sin rutas arbitrarias del
cliente, traversal, symlinks ni lectura de backing files. Usar aperturas relativas
seguras y un marcador ligado al esquema/servidor propio, sin secretos.

Un proceso escritor por raíz mediante bloqueo exclusivo; concurrencia interna
acotada. Temporales y objetos finales en el mismo filesystem. Buffer acotado,
cómputo MD5/SHA-512 durante escritura y límite de bytes incluso con longitud
desconocida. Controlar capacidad y reservas concurrentes sin prometer que una
comprobación previa impida ENOSPC posterior.

Protocolo de upload previsto:

1. Validar identidad, recurso, formatos, media type y declaraciones de tamaño.
2. En una transacción SQL corta, autorizar y reservar intento; queued → saving.
3. Escribir temporal exclusivo fuera de SQL; contar, hashear y validar longitud.
4. Sincronizar archivo y directorio temporal; persistir fase staged y resultado.
5. Bajo exclusión breve por imagen, verificar otra vez intento, estado y tombstone.
6. Publicar atómicamente sin sobrescritura y sincronizar directorios origen/destino.
7. Registrar published; confirmar active y committed mediante condición del intento.
8. Solo después del commit final confirmado, responder 204.

Un rename por sí solo no garantiza durabilidad tras una caída. No mantener una
transacción ni un bloqueo SQL durante la recepción del cuerpo. MySQL y filesystem
no comparten una transacción atómica: existen ventanas entre publicación y commit,
resueltas por intención durable, fencing y recuperación, no por un supuesto rollback
del filesystem efectuado por MySQL.

Para download: autorizar, abrir un descriptor seguro y comprobar objeto/tamaño
antes de responder. La propuesta verifica también hashes antes de emitir bytes,
con una pasada streaming adicional; añade I/O, no RAM proporcional a la imagen.
Un descriptor ya abierto puede terminar su lectura después de unlink concurrente
en Linux; nuevas consultas tras el tombstone devuelven 404. Un fallo durante la
respuesta termina la transferencia y se registra; nunca se añade un error JSON.

## J. Intenciones, fencing y eliminación

Cada imagen tiene un intento vigente identificado por un UUID nuevo. Las fases
previstas son reserved, staged, published, committed, cleanup y cleaned. Toda
publicación, finalización y restauración compara intento, fase, estado y deleted.
No reemplazar la intención anterior hasta confirmar cleaned; nunca sobrescribir
un objeto existente al reutilizar una imagen queued.

DELETE comparte la exclusión breve de publicación. Confirma juntos tombstone e
intención cleanup, destino deleted. Después cancela el escritor y elimina las
claves exactas del intento, sincroniza directorios y marca cleaned. No esperar
al escritor mientras se mantiene el mutex de publicación o un bloqueo SQL: se
evita el interbloqueo con su propia finalización/restauración.

Si DELETE confirma primero, un upload que detecta la eliminación termina con
410 si puede responder. Si el upload confirma primero, puede devolver 204 y
DELETE eliminar después; no producir un 410 retrospectivo. Una imagen ya
eliminada al comenzar una petición sigue devolviendo 404.

Un fallo de limpieza tras tombstone devuelve 503 en el diseño; GET ya es 404 y
la intención permanece. DELETE repetido conserva 404 de B20: ese 404 no acredita
borrado físico ni elimina el trabajo pendiente. Conservar propiedades/tags del
tombstone y el UUID reservado; no hay purga automática del catálogo.

## K. Fallos, commits inciertos y reinicios

| Frontera | Estado persistente posible | Resolución prevista |
| --- | --- | --- |
| Antes de reserva | queued, sin objeto | Devolver error sin efectos |
| Reserva SQL incierta | Puede existir reserved/saving | Releer antes de escribir; indisponibilidad no prueba rollback |
| Escritura interrumpida | reserved/saving y temporal parcial | Intención cleanup a queued; limpiar antes de restaurar |
| fsync realizado, staged aún no confirmado | Temporal completo y reserva | Tratar como intento no confirmado, no como active |
| staged confirmado | Temporal durable y hashes registrados | Tras caída, limpiar/restaurar; no promover automáticamente |
| Publicación hecha, fase aún incierta | Objeto final y metadatos anteriores | Resolver SQL e identificar el intento antes de actuar |
| Commit final incierto | Puede existir active/committed | Releer autoritativamente; si está confirmado, conservar objeto |
| Tombstone incierto | Puede haber imagen activa o deleted | No hacer unlink hasta confirmar tombstone e intención |
| Tombstone confirmado, unlink falló | deleted/cleanup y archivo | Reintentar limpieza idempotente de esa clave |
| Unlink hecho, cleaned no confirmado | Intención cleanup, archivo ausente | ENOENT es válido para la clave registrada; confirmar cleaned |

Resolver incertidumbre desde una conexión/transacción nueva al servidor
autoritativo, sin caché ni snapshot anterior. Una lectura bloqueante del registro
debe serializarse con la transacción previa. Si no puede resolverse, conservar
archivos/intenciones y devolver 503; no borrar a ciegas ni afirmar éxito.

La compensación destructiva exige primero una intención cleanup confirmada que
invalide una finalización tardía. La recuperación no modifica un active confirmado
por deducir que el cliente no recibió respuesta. Todo cambio usa condiciones del
intento vigente; una restauración antigua no revierte DELETE ni una reserva nueva.

Al iniciar, adquirir el bloqueo exclusivo de raíz antes de reconciliar y servir
tráfico. No usar solo un TTL para declarar abandonado un escritor vivo. Un intento
incompleto sin DELETE se limpia/restaura; un tombstone se limpia sin restauración;
active/committed se conserva y verifica. Una contradicción no resoluble se señala
y bloquea para revisión. No barrer ni borrar archivos desconocidos automáticamente.
La recuperación usa contextos propios acotados; no depende de que el contexto de
una petición ya cancelada siga vivo. El trabajo no completado persiste tras salida.

## L. Migración 002 prevista, no creada

Añadir una tabla InnoDB `image_data_operations` con una fila vigente por imagen:

| Campo | Tipo/condición previstos | Propósito |
| --- | --- | --- |
| image_id | CHAR(36) ascii_bin, PK/FK a images.id, sin cascada | Asociar intención y conservar tombstone |
| attempt_id | CHAR(36) ascii_bin, NOT NULL, UNIQUE | Identificar intento y derivar claves internas |
| phase | VARCHAR(16), NOT NULL, CHECK de las seis fases | Control del protocolo y recuperación |
| expected_size | BIGINT nullable, no negativo | Longitud esperada cuando se conoce |
| size / virtual_size | BIGINT nullable, no negativos | Resultado preparado; virtual_size puede ser null |
| checksum | CHAR(32) ascii_bin nullable | MD5 hexadecimal del contenido completo |
| os_hash_value | CHAR(128) ascii_bin nullable | SHA-512; algoritmo seleccionado sha512 |
| cleanup_target | VARCHAR(8) nullable, CHECK queued/deleted/killed | Destino de recuperación; obligatorio cuando phase=cleanup |
| last_error_code | VARCHAR(32) nullable | Categoría controlada, sin secretos ni payload |
| created_at / updated_at | DATETIME(6), NOT NULL, UTC | Trazabilidad del intento |

Índice previsto por phase/updated_at. Las fases staged/published/committed requieren
size y hashes no nulos; cleanup requiere destino válido. Validar esas invariantes
en dominio, SQL y verificación estructural. La finalización guarda los campos
calculados en images y el estado committed de la intención en una misma transacción.

Extender el migrador a un registro ordenado de versiones 1 y 2, checksums por
versión, rechazo de huecos/versiones desconocidas y dirty. Conservar autocommit
comprobado, advisory lock e inspección InnoDB del historial. MySQL DDL no es una
transacción reversible: un fallo queda dirty para revisión, sin reparación
destructiva automática. serve verifica pero no ejecuta migraciones.

No alterar 000/001 ni recalcular su contenido. El checksum SHA-256 de 001 permanece
`ba6a24fd786cca2198c49e7e88d49ed0eebf910d84bef1e6557049dc9513690b`.
Cuenta runtime con CRUD solo en el esquema propio; DDL reservado a la cuenta
migradora. No debilitar CheckAccount ni ValidateTestDSN ni modificar grants ajenos.

## M. virtual_size por formato

La captura raw de 20 bytes informa size=20 y virtual_size=20. Para raw se selecciona
igualdad con los bytes físicos. Esto no justifica esa igualdad para otros formatos.

Para QCOW2 se prevé un inspector acotado de cabecera, versiones explícitas y
validación de sus campos. Nunca ejecutar una conversión, cargar todo el archivo
ni abrir backing files externos. Los detalles de validación necesitan fixtures
de formato; el tratamiento upstream exacto no se ha comprobado en esta acción.

Para otros formatos aceptados por B20 sin inspector seguro, devolver virtual_size
null y documentar la diferencia; no inventar size virtual ni rechazar un formato
solo por restricciones de RAM. Un estado active no demuestra capacidad de boot.

## N. Pruebas y aceptación de acciones posteriores

Todas las comprobaciones de esta tabla están **pendientes para B25**. Una prueba
con dobles no se informa como MySQL, filesystem o Keystone real.

| Nivel | Casos y criterio observable |
| --- | --- |
| Unitarias de dominio/servicio | 400 formatos/tamaño; 413 límite/store lleno; clasificación recuperable sin killed genérico; permisos y fencing |
| Filesystem real temporal | Streaming, bytes idénticos, MD5/SHA-512, permisos, no sobrescritura, traversal/symlinks, publicación y limpieza idempotente |
| Errores inyectados | Escritura corta, error de sync/publicación, ENOSPC simulado, cancelación; etiquetar claramente el doble |
| MySQL real aislado | Esquema nuevo, v1→v2, idempotencia, dirty/checksum, transacciones y commit incierto; nunca DB original |
| HTTP | PUT 204; GET 200/204/206; Range múltiple 400 e inválido 416; chunked, tamaños, media type y límites |
| Concurrencia | Dos uploads, upload/PATCH, upload/DELETE con 410 cuando DELETE gana, GET/DELETE; sin pérdida ni resurrección |
| Restauración | Limpiar antes de queued; si limpieza falla, bloquear nueva reserva; callbacks viejos no afectan nuevos intentos |
| Persistencia y caídas | Reinicio efectivo del proceso y fallos en cada frontera; conservar committed y no promover incompletos |
| DELETE | protected, repeated 404, tombstone, fallo físico, reintento de limpieza; nunca unlink ante commit no resuelto |
| Corrupción | Ausencia real de contenido da 204; archivo confirmado perdido/corrupto da 503 propuesto |
| Regresión B20 | Suite completa: POST/GET/LIST/PATCH/DELETE de metadatos, serialización, nulabilidad, hashes protegidos y autorización |

Un ENOSPC real requiere posteriormente filesystem desechable de capacidad
limitada y autorización específica. No usar esquemas A ni uno contaminado por
DDL parcial para casos normales. Conservar las evidencias futuras fuera de S2,
sin tokens, DSN ni contraseñas. Preparar o compilar una prueba no equivale a
ejecutarla; un gate documental no valida streaming ni durabilidad física.

Criterios de aceptación: ningún 204 de upload antes de confirmar archivo y DB;
ningún active con bytes parciales; longitudes/hashes correctos; ninguna restauración
sobre tombstone; ningún intento nuevo sobre limpieza pendiente; lectura tras
reinicio; eliminación recuperable y todas las regresiones B20 aprobadas. Los
detalles NV requieren fuente/fixture o decisión explícita antes de afirmar
compatibilidad. B26 y las mediciones de rendimiento tienen aceptación separada.

## O. Exclusiones expresas

No hay multipart ranges, ETag/If-Range, upload reanudable/multipart, importación,
conversión de formatos, acceso a backing files externos, multi-store, replicación,
membresías nuevas, purga automática del catálogo, sustitución del original,
integración Nova B26 ni benchmarks. No se modifican Placement o la PR #3.
Este documento no habilita ningún endpoint ni autoriza ejecutar las acciones
posteriores. La Acción 1/8 termina con documentación y comprobaciones locales.
