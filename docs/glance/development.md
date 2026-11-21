# Glance Go — fundamento de B20 (Entrega A)

Esta guía conserva la fundación A y su reproducción. La [Entrega B](b20-catalog.md)
añade LIST/PATCH/DELETE/esquemas y define sus nuevas pruebas y límites. Los
resultados reales de A no validan automáticamente B.

Actualización B25: [operación, configuración y pruebas opt-in](b25-operations.md).
Los límites de transferencia/store descritos abajo corresponden a la Entrega A
histórica; el CLI puede ahora conectar B25 explícitamente. La integración binaria
real con 002/Keystone sigue pendiente y no se deduce de la evidencia B20.

Alcance original de A: POST `/v2/images` y GET
`/v2/images/{id}`. A no incluía listado, PATCH, DELETE ni esquemas auxiliares (añadidos en B).
Siguen pendientes discovery, transferencia, store, integración Nova y sustitución
del original. La aceptación operativa de B20 requiere validar la Entrega B. Las capturas originales de Sprint 2 permanecen intactas.

## Compilación y pruebas aisladas

Módulo raíz compartido: `github.com/PaoloCastanon/modules-devstack-optimized`.
Directiva mínima Go 1.24.0; objetivo de validación Go 1.27.3. Las dependencias
directas están fijadas: mysql v1.10.1 y google/uuid v1.6.0; `go.sum` fija los hashes.
`GOTOOLCHAIN` selecciona una toolchain en la caché del usuario, sin instalación
global. La primera ejecución necesita acceso a los servidores de módulos de Go.

Desde la raíz:

```sh
make glance-fmt-check glance-vet glance-test glance-build
```

Para una comprobación adicional con el Go instalado, sin descarga de toolchain:

```sh
make GLANCE_TOOLCHAIN=local glance-vet glance-test glance-build
```

Los tests normales usan `httptest`, dobles de autenticación y repositorio, un
doble del protocolo SQL y el adaptador Keystone contra servidores HTTP de prueba. No conectan a MySQL ni al
laboratorio. Aprobarlos no demuestra persistencia SQL ni integración Keystone real.
El test de representación compara el JSON de creación y el error 404 con las
capturas históricas existentes, sin sobrescribirlas. No hay benchmark ni prueba
de carga. El paralelismo de compilación está limitado a uno y GOMAXPROCS a dos.
El build usa `-trimpath -buildvcs=false`; no incorpora estado Git ni rutas locales.
Las invocaciones Make establecen `GIT_OPTIONAL_LOCKS=0` sin cambiar configuración Git.

## Arquitectura

`cmd/glance` compone configuración, adaptador Keystone, servicio y repositorio
MySQL. `api` resuelve HTTP/JSON; `service` aplica permisos y negocio; `model`
define atributos y restricciones; `repository` contiene el contrato mínimo de
crear/consultar. MySQL implementa SQL parametrizado con contextos y transacciones.
Se usa net/http, encoding/json, database/sql y log/slog, sin ORM ni framework.

No hay autenticador falso en el ejecutable ni opción para desactivar validación.
Las identidades de prueba solo existen en archivos `_test.go`.

## Configuración

Glance Go tiene un solo modo: sirve el catálogo, la base y el store del Glance
original, como su reemplazo. Lee el mismo archivo que el original,
`/etc/glance/glance-api.conf` (`GLANCE_GO_CONFIG_FILE`, obligatoria), con las
reglas de oslo.config:

| Sección del original | Qué toma Glance Go |
| --- | --- |
| `[database] connection` | Servidor, cuenta y base `glance` (catálogo) |
| `[glance_store] filesystem_store_datadir` | Directorio de los archivos `<id>` |
| `[keystone_authtoken]` | URL de Keystone, usuario de servicio y `token_cache_time` |
| `[oslo_messaging_notifications]`, `[DEFAULT]` | Notificaciones y `transport_url` |

Las variables que repiten algo de ese archivo (`GLANCE_GO_DATABASE_DSN_FILE`,
`GLANCE_GO_KEYSTONE_*`, `GLANCE_GO_NOTIFICATION_*`, `GLANCE_GO_TOKEN_CACHE_TIME`,
`GLANCE_GO_OFFICIAL_STORE_DIR`, etc.) se rechazan: el archivo del original es la
única fuente. Las variables propias de Glance Go son solo lo que el original no
define (ver `configs/glance.env.example`):

| Variable | Valor predeterminado / función |
| --- | --- |
| `GLANCE_GO_CONFIG_FILE` | Obligatoria: el `glance-api.conf` del original |
| `GLANCE_GO_LISTEN_ADDR` | `127.0.0.1:9293`; requiere host explícito |
| `GLANCE_GO_PUBLIC_BASE_URL` | `http://127.0.0.1:9293`; base de Location |
| `GLANCE_GO_BINARY_ENABLED`, `GLANCE_GO_STORE_ROOT`, `GLANCE_GO_STORE_SCOPE` | Raíz privada (lock y marca de alcance) |
| `GLANCE_GO_REQUEST_TIMEOUT` | `15s`; >0 y <=1m |
| `GLANCE_GO_DEPENDENCY_TIMEOUT` | `5s`; menor que el plazo de petición |
| `GLANCE_GO_MAX_BODY_BYTES` | `1048576`; límite operativo de metadatos |

Glance Go usa la misma cuenta MySQL que el original y guarda su única tabla
propia, `image_data_operations` (contabilidad de transferencias), en la base
`glance_go_shared` del mismo servidor. Esa tabla no tiene FK hacia `images`,
porque `images` vive en `glance`. La conexión conserva las reglas de transporte:
TCP sin TLS solo con IP loopback literal o socket Unix, TLS verificado en otro caso,
y se rechazan `allowFallbackToPlaintext`, `allowOldPasswords`,
`allowCleartextPasswords`, `allowAllFiles`, `tls=skip-verify`, `tls=preferred` y
multiStatements. Como los dialectos MySQL de SQLAlchemy, cuenta las filas
encontradas (`CLIENT_FOUND_ROWS`), no las cambiadas: los `DATETIME` del original
tienen segundos, y un PATCH solo de propiedades dentro del mismo segundo deja la
fila `images` igual.

`Location` se construye desde la base configurada; no confía en Host ni en
X-Forwarded-*. Solo se admiten HTTP/HTTPS, un host DNS ASCII o IP válido y un
puerto válido cuando se indica. Se rechazan credenciales, query/fragmento (también
los separadores `?`/`#` vacíos), escapes `%`, segmentos `.`/`..`, barras interiores
duplicadas y barras invertidas. Se permiten bases como `https://host/image/` y
barras finales repetidas; se normalizan al unir componentes con `URL.JoinPath`.

## Esquema y migraciones

Glance Go no migra el catálogo del original: al arrancar exige que
`glance.alembic_version` sea una revisión revisada (`2024_1_contract01`) y, con
otra, no sirve. `glance migrate` crea o verifica solo `glance_go_shared`:
`000_history.sql` inicializa `schema_migrations` y la versión 1
(`001_image_data_operations.sql`) crea la tabla de transferencias.

Un bloqueo MySQL de sesión impide migradores concurrentes. Inspección, bloqueo,
DDL e historial usan una única conexión reservada con `@@SESSION.autocommit=1`
comprobado. La marca dirty se persiste antes del DDL y solo se limpia al
finalizar; la actualización final exige una fila con el checksum esperado y se
relee el historial antes de devolver éxito. MySQL hace commits implícitos en DDL:
ante un fallo, detener y revisar a mano; no hay down ni recuperación destructiva
automática. `serve` verifica versión, checksum, dirty y la estructura exacta de
`image_data_operations`, y nunca ejecuta DDL.

Al arrancar, Glance Go adopta las imágenes que subió el original (registra su
operación confirmada a partir de los hashes verificados y su única ubicación
local) y concilia su contabilidad con lo que hizo el original:
- si el original borró una imagen que subió Glance Go, completa la limpieza como
  en un borrado propio (borra el archivo si quedó y marca la operación limpia);
- si `glance-manage db purge` eliminó la fila de la imagen, borra el archivo
  sobrante, si lo hay, y la fila huérfana de `image_data_operations`.

## Integración MySQL (`make glance-test-integration`)

Las suites de integración corren contra el MySQL de la VM del laboratorio, el
mismo que usa el original, por un túnel SSH (el DSN queda en loopback, como exige
Glance Go). `scripts/glance_integration_tests.sh`:
- lee en la VM la cuenta del original desde `/etc/glance/glance-api.conf` (la
  contraseña solo queda en un archivo temporal privado);
- copia el DDL real de `glance` (sin filas) y su revisión alembic a un esquema
  `glance_official_test_<suite>` por suite, junto a un `glance_go_shared_test_<suite>`
  vacío;
- corre las suites (persistencia y transacciones, historial, DDL parcial,
  transferencias, catálogo, borrado coordinado, fences y contención B25, y la B25
  de punta a punta HTTP + MySQL + store);
- borra todos los esquemas de prueba al salir, también si algo falla, y revisa
  que los logs no contengan la contraseña.

```sh
make glance-test-integration                       # VM encendida
GLANCE_INTEGRATION_RACE=1 make glance-test-integration   # con -race
```

El build tag `integration` las separa del gate normal; sin el opt-in que pone el
script, fallan en vez de saltarse. `glance` del original solo se lee (DDL y
revisión).

## Keystone real

El adaptador obtiene un token de servicio con el usuario de servicio del original
(`[keystone_authtoken]`, autenticación por contraseña, como keystonemiddleware) y
valida el token cliente con GET `/v3/auth/tokens?nocatalog`: X-Auth-Token identifica
al servicio; X-Subject-Token al usuario.

El token propio se guarda en memoria hasta su renovación. Desde 2026-11-18 los
tokens entrantes validados con éxito también se reutilizan durante
`GLANCE_GO_TOKEN_CACHE_TIME` (300 s por defecto), igual que keystonemiddleware en
Glance oficial ([performance.md](performance.md)). Placement Go no usa esta caché. La renovación comparte únicamente el resultado del
token de servicio mediante un canal de finalización; no mantiene un mutex durante
HTTP ni crea goroutines propias. Cada solicitud en espera puede cancelar su contexto.
Si quien renueva falla o vence, quienes esperaban reciben ese fallo; una petición
posterior puede renovar de nuevo. Las identidades cliente nunca se comparten.
No se siguen redirects que puedan reenviar secretos.
En autenticación simple se rechazan tokens vencidos o malformados, scope sin
proyecto y respuestas inválidas.
Un 404 de validación significa token inválido; un 401 del validador renueva el token
del servicio una sola vez. Errores de configuración/red/servidor producen 503 y no
permiten acceso. No se confía en X-Roles, X-User-Id ni X-Project-Id del cliente.
N2 añade validación independiente de X-Service-Token cuando está presente, sin
transferir sus roles al usuario. El servicio debe tener rol service, un token
vigente y el mismo proyecto que la credencial validadora de Go. El usuario puede
pertenecer a otro proyecto. La excepción para un usuario vencido depende de una
validación nueva de Keystone con allow_expired=true, nunca de un plazo local ni
de omitir la revocación. Ver [contrato N2](composite-auth.md), incluidos sus límites
y las pruebas operativas todavía pendientes.

HTTP sin token/incorrecto/vencido sin delegación válida produce 401;
identidad sin proyecto o permiso produce 403;
imagen privada/shared de otro proyecto produce 404. Crear requiere member o admin;
publicar requiere admin. Consultar requiere reader o admin y acceso por propietario
o visibilidad public/community. Se utilizan los roles efectivos del token Keystone,
sin inventar jerarquías locales. No se implementan membresías de imágenes shared.

La integración con Keystone real está **NO VERIFICADA** hasta ejecutar pruebas
autorizadas con esa identidad. Tests de protocolo no equivalen a integración real.
Las application credentials restringidas mediante access_rules no están soportadas:
no se anuncia la cabecera de soporte ni se implementa su enforcement.

## Contrato, diferencias y pendientes

POST devuelve 201 con estado queued, UUID generado o indicado, owner del proyecto,
visibility shared por defecto, mínimos cero, flags falsos, tags [], campos calculados
null y propiedades al mismo nivel. GET devuelve el objeto directamente y 404 si
no existe/no es visible. JSON inválido/tipos incorrectos dan 400; campos reservados
403; conflicto UUID 409; media type 415; cuota/cuerpo excesivo 413; dependencia
indisponible 503. El cuerpo completo debe ser UTF-8 válido antes de decodificar JSON;
bytes inválidos producen 400, sin sustitución silenciosa. Unicode válido y propiedades
extra conservan sus valores. Primero se aplica el límite del cuerpo (413). El texto
del error UTF-8 no tiene fixture Python y no se declara idéntico al original.
Errores usan message/code/title, incluyendo el sufijo observado
en el fixture 404. Nunca se devuelve ni registra SQL, DSN, token o cuerpo secreto.

Cada petición registra un evento estructurado con request_id generado por el
servidor, método, plantilla de ruta, estado HTTP, duración y categoría de error.
Los fallos de identidad/metadatos tienen categorías separadas y los 5xx nivel ERROR.
No se registran rutas arbitrarias, UUID de imagen, queries, cabeceras de identidad,
tokens, cuerpo o mensajes crudos del driver. El fallo al escribir la respuesta queda
categorizado sin cambiar la respuesta HTTP. El gate de formato comprueba el exit
code de gofmt; sus regresiones usan archivos temporales fuera del repositorio.

Ausente y null convergen solo en nombre y formatos. null no es válido para UUID,
owner, visibility, mínimos, flags, tags o propiedades extra. Nombre vacío y propiedad
vacía permanecen vacíos; cero/falso/[] permanecen presentes. UUIDs se canonizan a
minúsculas; no se exige versión 4 a UUIDs indicados, solo a los generados. Los formatos
permitidos siguen las listas del original auditado, aunque A no procesa sus bytes.

Diferencias deliberadas/limitaciones que deben verificarse contra nuevos fixtures:

- Owner explícito solo si coincide con el proyecto; no override administrativo ni
  owner null. No protege propiedades mediante reglas configurables por atributo.
- Se exige JSON entero para mínimos; no números decimales equivalentes como 1.0.
- Propiedades extra son cadenas; no se incluyen excepciones de schema-image.json
  para propiedades tipadas/anulables como kernel_id.
- Nombre/tag/clave hasta 255 caracteres; valor de propiedad hasta 65535 bytes;
  cuotas fijas 128 tags únicos/propiedades y límite de cuerpo configurable. Son
  límites de esta entrega, sin equivalencia negativa completa demostrada.
- No se anuncian métodos de importación ni soporte completo v2.18. Discovery sigue pendiente; schema, listado, PATCH y DELETE están en B. No afirmar compatibilidad completa de cliente.
- No se replica el puerto loopback del Location histórico ni Server/Date de Apache.
- La cabecera 401 se corrige y contrasta en B; el cuerpo conserva JSON frente al
  texto original. Mensajes negativos adicionales y negociación Accept requieren
  fixtures antes de afirmar equivalencia. El 404 histórico y la creación sí se
  comparan automáticamente a nivel de representación JSON.
- La Entrega A se validó contra MySQL/Keystone reales el 27/10/2026, incluido
  DDL parcial e historial tras reinicio del servidor temporal. Las nuevas rutas
  de B requieren su propia integración; no se ha probado recuperación ante
  corte de energía ni se extrapolan los resultados funcionales a rendimiento.

No usar `make baseline`, el wrapper antiguo de compatibilidad o `make smoke` para
validar A: modifican datos/evidencias del original o dependen del ciclo binario.
La [Entrega B](b20-catalog.md) implementa listado, PATCH, DELETE y esquemas
auxiliares; su integración real y fixtures diferenciales adicionales siguen pendientes. B25/B26 y benchmarks permanecen fuera.
