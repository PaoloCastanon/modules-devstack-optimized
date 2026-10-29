# B07 — Primer incremento providers/inventories

Estado al 2026-10-28: implementado y probado localmente con MySQL real.
**B07 continúa en progreso**: faltan comparación diferencial en vivo, fixtures
negativos adicionales y la ampliación de microversiones necesaria para Nova.

## Contrato implementado

El proceso `cmd/placement` usa el módulo Go integrado por el PR #3 y separa API,
servicio, modelo y repositorio. No importa el dominio Glance. `internal/config`
comparte el lector de configuración con prefijos independientes; la configuración
Glance conserva su interfaz y defaults. `internal/auth` valida tokens Keystone.

Se anuncia únicamente **Placement 1.0**, para las rutas seleccionadas siguientes.
No representa una implementación completa de toda la API de esa microversión.
Un cliente que pida 1.36 recibe 406; todavía no se puede sustituir Placement
usado por Nova. No se anuncian 1.39 ni las capacidades del servidor original.

| Método | Ruta | Resultado seleccionado |
|---|---|---|
| GET | `/` | Discovery 200 sin autenticación, min/max 1.0 |
| POST | `/resource_providers` | name obligatorio, UUID opcional; 201 vacío con Location |
| GET | `/resource_providers` | Array, filtros name/uuid por igualdad y AND |
| GET | `/resource_providers/{uuid}` | Provider con generation y links de 1.0 |
| PUT | `/resource_providers/{uuid}` | Renombrar; 200; no recibe ni incrementa generation |
| DELETE | `/resource_providers/{uuid}` | 204 vacío, elimina también sus inventories |
| GET | `/resource_providers/{uuid}/inventories` | generation y mapa, incluso vacío |
| PUT | `/resource_providers/{uuid}/inventories` | Reemplazo atómico; exige generation; incrementa una vez |
| GET | `/resource_providers/{uuid}/inventories/{class}` | Inventory o 404 |

No hay POST/DELETE de inventory individual, árbol, aggregates, usages, traits,
catálogo HTTP de clases, allocations ni candidates todavía. Los enlaces de
provider reproducen la representación 1.0, incluido usages, pero ese recurso
aún no tiene implementación. El catálogo inicial contiene solo VCPU, MEMORY_MB
y DISK_GB, observados en S2; B08 debe ampliarlo. No hay importación de datos
del original, ni escritura sobre sus esquemas.

Fuentes de contrato: fixtures S2 y código Placement fijado en
`f4a89d3803cb23df22b8714569889ab39c918c34`, concretamente handlers, schemas,
objetos y políticas de providers/inventory. [Referencia upstream](https://github.com/openstack/placement/tree/f4a89d3803cb23df22b8714569889ab39c918c34/placement)
y [API oficial](https://docs.openstack.org/api-ref/placement/).

## Reglas y límites de compatibilidad

- El nombre admite hasta 200 caracteres, incluido vacío según schema upstream.
  Al crear, UUIDs compactos o con guiones se canonizan; los lookups usan el valor
  recibido sin reescribirlo, como el código fijado. Inválido en create/query da 400 y
  en consulta de recurso da 404.
- Inventory requiere total; defaults: reserved=0, min_unit=1,
  max_unit=2147483647, step_size=1, allocation_ratio=1.0. Los enteros son de
  32 bits no negativos, con mínimo 1 salvo reserved. La capacidad truncada debe
  ser positiva en 1.0; reserved=total queda rechazado. No se impone min_unit<=max_unit
  como validación adicional, porque ese schema no lo exige.
- Los campos JSON son sensibles a mayúsculas. Null, miembros desconocidos y
  tipos incompatibles dan 400. Los mínimos requieren sintaxis JSON entera: `1.0`
  en un campo int no se acepta en este incremento. Duplicados JSON conservan el
  último valor, como el decodificador de referencia; queries repetidas se rechazan
  aunque versiones tempranas del original puedan elegir un valor.
- Solo admin o service con scope de proyecto pueden usar providers/inventories,
  según las políticas fijadas. No se aplican permisos owner/member de Glance.
  X-Roles/X-Project-Id del cliente no aportan identidad. Sin token/expirado: 401;
  rol insuficiente: 403; validación de identidad indisponible: 503. La credencial
  del validador es independiente del token del cliente. X-Service-Token compuesto
  sigue rechazado hasta verificar su semántica en B12.
- Los errores conservan el contenedor `errors`, status numérico, title y
  request_id. El 404 de provider se contrasta con el fixture histórico. Los otros
  mensajes negativos todavía no tienen equivalencia completa demostrada.
- Los filtros name y las restricciones únicas usan collation utf8mb4_0900_bin;
  la equivalencia de mayúsculas/espacios con la DB original queda pendiente.
  allocation_ratio usa DOUBLE; quedan pendientes casos diferenciales de precisión.
- Los límites de body/query y la negociación Accept tienen regresiones locales;
  no se afirma equivalencia completa de prioridades entre errores y middleware.

La DB actual no contiene allocations ni las acepta por HTTP. Por tanto, aún no
se puede probar el rechazo de borrar/reducir inventario en uso; B09 debe agregar
esas reglas **antes** de admitir reservas, conservando el bloqueo del provider.

## Persistencia y operación

Esquema propio `placement_go` o `placement_go_test_<sufijo>`. Configuración en
[placement.env.example](../../configs/placement.env.example), con archivos de
secretos privados 0600. DSN de aplicación separado del DSN migrador. El programa
no crea DB ni usuarios, y no imprime DSN, tokens, SQL o errores crudos de drivers.
La cuenta de ejecución requiere SELECT/INSERT/UPDATE/DELETE solo en su esquema;
la migradora añade CREATE/ALTER/INDEX/REFERENCES. Sin permisos globales, DDL en
la cuenta HTTP, roles delegados ni GRANT OPTION. Como en Glance, el guard verifica
aislamiento y permisos excesivos; el operador debe comprobar que no faltan grants.

MySQL remoto exige TLS verificado; plaintext solo en loopback literal o socket.
Los nombres de DB y las cuentas originales/admin están bloqueados. Los grants
deben escapar `_` si partial_revokes está OFF. No cambiar esa variable global.
Las guardias pertenecen al repositorio Placement; no se importa el paquete MySQL
Glance, cuyas políticas de esquema y migraciones son distintas.

Migración explícita v1 con checksum, historial InnoDB, dirty persistido antes de
DDL y lock de sesión. Un fallo deja dirty; no hay reparación destructiva automática.
Serve solo verifica el historial; no ejecuta DDL ni inspecciona toda posible
alteración manual de las tablas de dominio. El esquema v1 queda fijado: futuras
ampliaciones requieren nuevas migraciones, sin reescribir su checksum.

Todas las mutaciones bloquean el provider. Replace hace CAS de generation,
reemplaza los hijos y confirma en una sola transacción. Una lectura de inventories
usa snapshot consistente. El fallo SQL del segundo hijo se prueba con una FK real,
verificando que también se revierta el incremento de generation. La persistencia
se comprobó al reabrir pool y al reiniciar el proceso; no se reinició el servidor
MySQL ni se simuló un corte de energía.

```sh
make placement-build
# Entorno exportado desde una configuración privada revisada:
PLACEMENT_GO_DATABASE_DSN_FILE=/absolute/private/migrate.dsn ./bin/placement migrate
PLACEMENT_GO_DATABASE_DSN_FILE=/absolute/private/app.dsn ./bin/placement serve
```

El listener por defecto es 127.0.0.1:8779. PUBLIC_BASE_URL controla Location y
el prefijo de links; Host/X-Forwarded-* no los controlan. Si la base contiene
`/placement`, el proxy debe retirar ese prefijo al reenviar. Hay timeouts HTTP/DB
y cierre por SIGTERM/SIGINT. No hay opción desplegable de autenticación falsa.

## Pruebas reproducibles y evidencia

```sh
make check
make placement-test
```

Los tests de API usan un repositorio en memoria exclusivamente de pruebas y
comparan las respuestas con fixtures archivados de provider-create/show,
inventory-put/show y provider-not-found. Solo eliminan request_id del 404 para
comparar su contenido estable. Las capturas S2 no se modificaron. Esto es
regresión contra capturas; **todavía no es un comparador en vivo Python↔Go**.

Para integración, preparar fuera del original una instancia MySQL 8.0 y **dos
esquemas nuevos vacíos**, uno por suite, con sus cuentas migrador/aplicación
dedicadas y DSN privados. Nunca apuntar al laboratorio ni reutilizar esquemas
dirty o de otra suite. Cada suite exige el opt-in y valida el destino; no hay
tests de integración dentro del gate local ni skips que simulen aprobación.

```sh
PLACEMENT_GO_MYSQL_TEST=1 \
PLACEMENT_GO_TEST_MIGRATE_DSN_FILE=/absolute/private/mysql-suite-migrate.dsn \
PLACEMENT_GO_TEST_APP_DSN_FILE=/absolute/private/mysql-suite-app.dsn \
make placement-test-mysql

# Otro esquema NUEVO VACÍO y otras cuentas limitadas a él:
PLACEMENT_GO_MYSQL_TEST=1 \
PLACEMENT_GO_TEST_MIGRATE_DSN_FILE=/absolute/private/binary-suite-migrate.dsn \
PLACEMENT_GO_TEST_APP_DSN_FILE=/absolute/private/binary-suite-app.dsn \
make placement-test-binary
```

La primera comprueba migración/idempotencia, cuentas, duplicados, CAS, rollback,
concurrencia, reapertura, DELETE y HTTP mediante el validador Keystone de producción.
Marca dirty al final para comprobar rechazo; conserva el esquema descartable.
La segunda compila el binario, ejecuta migrate/serve, crea mediante HTTP, envía
SIGTERM, reinicia y verifica los datos. **Ambas usan un servidor Keystone de
fixture**: no acreditan integración con Keystone desplegado.

La ejecución de esta tarea usó un contenedor descartable, puerto solo loopback,
datos tmpfs y MySQL 8.0.46 fijado por digest
`sha256:7dcddc01f13bab2f15cde676d44d01f61fc9f99fe7785e86196dfc07d358ae2b`.
Los resultados saneados están en [evidencia B07](../../evidence/sprint3/placement/b07/README.md).
La VM original permaneció apagada. No se modificó el Excel ni se acreditó boot
Nova, B11/B12 completos o el cierre formal B07.
