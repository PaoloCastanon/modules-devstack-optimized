# B20 — Entrega B: contrato seleccionado del catálogo

Esta entrega añade código y regresiones para LIST, PATCH y DELETE sobre el modelo
de la Entrega A. La integración real de A no valida automáticamente estas rutas.
La aceptación operativa de B requiere ejecutar la suite nueva en un esquema
independiente autorizado y completar HTTP con Keystone real. No afirmar B20
aceptado/completo basándose únicamente en pruebas aisladas.

## Fuentes y decisiones de compatibilidad

- [Contrato Sprint 2](../sprint2/glance-api-scope.md) y capturas originales
  `images.response.json`, `image-patch.request.body` y `image-patch.response.json`.
- [Image API v2 oficial](https://docs.openstack.org/api-ref/image/v2/).
- Lectura focalizada del Glance Python desplegado: controlador/deserializador de
  `glance/api/v2/images.py` y paginación de `glance/db/sqlalchemy/api.py`.
- La captura de 401 de la integración A se conserva sin cambios fuera del repo;
  sus cabeceras saneadas se copiaron a
  [glance-python-401.json](../../tests/compatibility/fixtures/glance-python-401.json)
  para una regresión portable. No contiene tokens.

El documento de Sprint 2 decía «filtros/paginación» sin enumerarlos. Esta entrega
concreta el subconjunto siguiente; no declara implementar todos los parámetros
ni toda Image API v2.18. El DELETE de Sprint 2 incluía store; aquí se respeta el
alcance B20 solicitado: solo metadatos sin bytes. Store queda para B25.

## Autenticación y 401

Todas las rutas mantienen validación Keystone real, scope de proyecto y rechazo
de cabeceras de identidad del cliente. LIST/GET requieren reader o admin;
POST/PATCH/DELETE member o admin. El propietario sigue siendo el proyecto cliente.

Un 401 incluye `WWW-Authenticate: Keystone uri="<endpoint identity>"`. Se usa la
URL Keystone configurada, validada, quitando el componente final `/v3`, tal como
la captura original. No se usa Host/X-Forwarded-* ni tokens. Se conserva el JSON
de error anterior: Python devuelve texto en el caso negativo capturado, por lo
que sigue existiendo esa diferencia de cuerpo/Content-Type, declarada aquí.

## GET /v2/images

Respuesta 200 con `images` (siempre array), `first`, `schema` y `next` cuando se
llena la página. Los enlaces son relativos `/v2/...`, conservan filtros repetidos
y ordenamiento; `first` omite marker. Igual que el original, una página de tamaño
exactamente limit anuncia next aunque la página siguiente esté vacía.

| Parámetro | Subconjunto implementado |
| --- | --- |
| limit | Entero >=0; por defecto 25, máximo efectivo 1000; 0 produce [] |
| marker | UUID canónico; exclusivo; inexistente, borrado o no accesible da 400 |
| sort | `name:asc,created_at:desc`; dirección omitida significa desc |
| sort_key / sort_dir | Sintaxis clásica repetida; una dirección se aplica a todas las claves |
| Claves de orden | name, status, container_format, disk_format, size, id, created_at, updated_at |
| Filtros de igualdad | id, name, status, owner, visibility, disk_format, container_format, protected, os_hidden, min_disk, min_ram |
| tag | Repetible; se deben cumplir todas las etiquetas |
| Propiedades adicionales | Igualdad de cadenas mediante parámetros SQL |
| member_status | Solo accepted, sin ampliar el acceso; membresías no implementadas |

Las claves de desempate `created_at` e `id` se añaden si faltan. La dirección
de desempate reproduce la selección del original: una única dirección clásica
se propaga; para múltiples direcciones independientes el fallback es desc.
NULL se ordena como MySQL (primero en asc, último en desc); los predicados marker
son explícitamente seguros para NULL y no confunden NULL con cadena vacía.
Esto evita las omisiones que puede producir el COALESCE del paginador Python
en el caso límite NULL/cadena vacía; no se promete equivalencia de ese defecto.

Antes de materializar resultados, SQL exige `deleted=false` y acceso por
propietario o visibilidad public/community. Sin filtro visibility, community
solo aparece si pertenece al proyecto consultante; `visibility=community` o all
permite ver las community ajenas. Shared ajena no es visible porque no existe
API de membresías. Admin puede ver imágenes privadas de otros proyectos.
Por defecto `os_hidden=false`; con true se piden solo las ocultas.

Se rechazan controles repetidos ambiguos, sort moderno mezclado con clásico,
direcciones/claves inválidas, UUID inválido, enteros negativos y filtros base
no soportados. No hay operadores de rango/listas (`gt`, `in`, etc.), size_min/max,
filtros temporales, búsqueda textual ni membresías pendientes/rechazadas. Los
valores de los filtros seleccionados se interpretan como igualdad literal; un
nombre desconocido no reservado se interpreta como propiedad adicional. Query
máxima 16 KiB, 128 filtros efectivos/etiquetas, límite numérico de 64 bits.

## PATCH /v2/images/{id}

Solo `application/openstack-images-v2.1-json-patch`; otros media types dan 415
con Accept-Patch. Se admite un array (incluso []) de hasta 128 operaciones add,
replace o remove. Se mantiene el límite de cuerpo configurado y UTF-8 válido.

- add reemplaza si el atributo ya existe, como la variante v2.1 del original.
- replace/remove de propiedad adicional inexistente: 409.
- remove de propiedad base: 403. Usar replace null en los campos anulables.
- Rutas de un solo token JSON Pointer; `~0` y `~1` permiten tilde/barra en claves
  adicionales. Se rechazan escapes inválidos, anidamiento e índices de tags.
- Mutables: name, disk_format, container_format, visibility, min_disk, min_ram,
  protected, os_hidden, tags y propiedades adicionales de cadena.
- id, owner, estado, hashes, tamaños, fechas y atributos reservados son inmutables.
  Owner permanece inmutable incluso para admin (limitación seleccionada de A).
- Solo admin puede publicar public; miembro de otro proyecto no puede modificar
  la imagen. Se conserva 404 para ocultar private/shared ajenas y 403 para una
  public/community ajena ya visible. Formatos solo se cambian en queued.
- No hay draft v2.0, test/copy/move, subrutas de locations ni miembros de arrays.
  Los miembros duplicados/desconocidos de un objeto operación se rechazan (400).

Se valida cada operación sobre una copia independiente. El callback de negocio
corre bajo `SELECT ... FOR UPDATE`; si cualquier operación falla, no se ejecuta
DML del cambio. Parent y sustitución de propiedades/etiquetas comparten una
transacción. Un fallo SQL posterior al UPDATE revierte también todas las tablas.
updated_at es UTC, con microsegundos persistidos y avance monótono; el JSON
mantiene segundos como las capturas. Dos PATCH concurrentes leen el estado
actual bajo el bloqueo, evitando perder cambios en propiedades distintas.
Conflictos de propiedad o deadlock/lock timeout dan 409; no se hacen reintentos
automáticos ni se incorpora ETag/If-Match en este subconjunto.

## DELETE /v2/images/{id}

Solo dueño member/admin o administrador. Protegida: 403, incluso admin. Imagen
inexistente, ya borrada o private/shared ajena: 404. Solo queued sin tamaños,
checksum o multihash de contenido; otro estado/contenido da 409 para no simular
el borrado coordinado de bytes. Éxito 204 sin cuerpo.

Bajo el mismo bloqueo y transacción se ponen deleted=true, status=deleted,
deleted_at y updated_at. Se conservan propiedades/etiquetas ligadas a la fila
tombstone y el UUID permanece reservado. GET y LIST excluyen tombstones. No
hay purga física, eliminación de archivos ni tareas de limpieza automática.

## Esquemas y persistencia

GET `/v2/schemas/image` y `/v2/schemas/images` sirven las representaciones del
subconjunto seleccionado que necesitan los enlaces y el cliente. Mantienen
autenticación; no anuncian toda Image API, importación, ubicaciones ni metadefs.
No sustituyen discovery/versiones o integración de clientes completos/Nova.

No hay nuevas migraciones: se reutilizan las columnas, índices y FK de v1,
sin alterar SQL ni checksum histórico. La cuenta de ejecución de B debe tener
SELECT/INSERT/UPDATE/DELETE sobre **su propio esquema**, sin DDL ni permisos
globales. La cuenta real de A gg_persist_run_b solo tiene SELECT/INSERT:
**no es suficiente para PATCH/DELETE**. No se modificaron sus grants. CheckAccount
comprueba aislamiento/permisos excesivos, no la disponibilidad de cada privilegio
funcional; el operador debe preparar y revisar una cuenta B adecuada.

## Pruebas y ejecución reproducible

Pruebas locales sin servicios reales:

```sh
GOMAXPROCS=2 GOTOOLCHAIN=local go test -p 1 ./...
GOMAXPROCS=2 GOTOOLCHAIN=local go vet -p 1 ./...
GOMAXPROCS=2 GOTOOLCHAIN=local go test -race -p 1 ./...
```

La suite opt-in nueva `TestMySQLCatalogOperations` requiere una DB **nueva vacía**
`glance_go_test_<sufijo>`, fuera de la VM original, con cuenta exclusiva y grants
directos SELECT/INSERT/UPDATE/DELETE/CREATE/ALTER/INDEX/REFERENCES solo sobre ella.
No ejecutar sobre los esquemas de A ni sobre el esquema dirty. No aprovisiona
cuentas, no cambia grants y deja los datos para revisión. Su autenticación de
políticas usa identidades de fixture explícitas; no demuestra Keystone real.

```sh
GLANCE_GO_MYSQL_TEST=1 \
GLANCE_GO_TEST_DSN_FILE=/absolute/private/new-catalog-test.dsn \
make GLANCE_TOOLCHAIN=local glance-test-mysql-catalog
```

Comprueba filtros, orden/páginas y visibilidad reales, PATCH válido, fallo de la
segunda operación sin cambio persistido, fallo SQL de hijo después del UPDATE,
PATCH concurrentes, lectura desde pool nuevo, protected, permisos, DELETE,
repetición, exclusión de listados y retención de propiedades/tags tombstone.
Solo debe ejecutarse después de autorizar y verificar esa infraestructura.
Compilarla no equivale a ejecutarla. La validación HTTP real con Keystone debe
usar luego un esquema/cuenta B autorizados y no modificar imágenes originales.
