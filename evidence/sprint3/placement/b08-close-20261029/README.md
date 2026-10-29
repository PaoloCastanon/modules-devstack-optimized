# Campaña B08 — 2026-10-29

Resultado final: **82/82** status, cabeceras estables, JSON y estados coinciden
entre Placement Python y Go para los casos seleccionados 1.1–1.7; regresión
B07 **57/57** en 1.0. Ambos runners terminaron en 0 y verificaron la limpieza.
No se acredita toda la API de las microversiones ni integración Nova.

## Procedencia y artefactos

- Base integrada B07: b4d8300977dac59ecd22ad5c1aff79590f643a08 (PR #6).
- Original Placement: f4a89d3803cb23df22b8714569889ab39c918c34.
- Nova inspeccionado: 247ea5d774305b688997f8df81c011b249eb793c; report.py usa
  traits 1.6 y ensure de resource classes 1.7. No se ejecutó Nova contra Go.
- Catálogos capturados de paquetes desplegados: os-resource-classes 1.1.0,
  21 nombres; os-traits 3.9.0, 377 nombres. Snapshot en tests/compatibility/fixtures.
- Go 1.27.3, MySQL 8.0.46 descartable y cuentas propias distintas para migrate
  y serve. Tokens admin/service/reader y validador de Keystone real, creados
  para el ensayo en proyecto/usuario temporales. No se almacenan secretos.
- `attempt-3/provenance.json`: hash del ejecutable y entorno. `source-files.sha256`
  identifica fuentes e inputs embebidos, porque la campaña precedió al commit.
- `attempt-3/run/report.json`: requests, respuestas completas, estados antes/después,
  diferencias, status esperado, generación esperada y cleanup. `b07-regression/`
  conserva los 57 casos anteriores sin cambiar su comparador.
- `attempt-3/restart-proof.json`: SIGTERM con exit 0 y nuevo proceso con la misma
  DB; persisten provider, clase CUSTOM, inventario, traits y asociaciones. No se
  reinició el servidor MySQL ni se simuló caída abrupta.
- `attempt-3/{catalog,binary,collision}-test.log`: upgrade v2→v3, rollback real,
  CAS traits/inventories, protección de catálogos; ciclo de ejecutable/migraciones
  y rechazo del upgrade v1 con colisión. `make-check.log`: gate local completo.
- `identity-cleanup.json` y `lab-final-state.json`: ausencia de proyecto/usuario
  temporal y VM apagada. Los contenedores, procesos locales y archivos privados
  también se eliminaron. No se sustituyeron endpoints ni servicios originales.

## Ensayos conservados y correcciones

`migration-red.log` conserva el error MySQL 1826: quitar y recrear una FK con el
mismo nombre en un ALTER falla. La migración v3 nueva separa las sentencias;
v1/v2 permanecen intactas. No se reintentó sobre el esquema dirty: cada prueba
usa otra DB vacía. El guard de historial incompleto sigue rechazando el arranque.

`attempt-1` obtuvo 75/75 status y 0/75 comparaciones estrictas: los snapshots
404 de clase/aggregates diferían en Vary; cuatro límites de versión y un 405
diferían en el detalle WebOb. `http-errors-red.log` reproduce esas diferencias
en tests antes de corregir; el fixture conserva las respuestas Python literales.
El comparador no se debilitó. El helper de campaña de ese ensayo además falló
al importar el módulo de restart; el finally sí limpió procesos, DB y secretos.

`attempt-2` obtuvo 75/75 estricto y B07 57/57. `attempt-3` amplía a 82 casos
por las comprobaciones pendientes de renombre en uso, duplicados y precedencia
cuando falta el provider; también verifica restart con los nuevos catálogos.

## Comparación y límites

Se conserva la normalización B07 de request_id, origen de Location y tokens Vary.
El runner B08 además ordena listas de traits/aggregates y clases (sin deduplicar),
porque upstream no fija orden. Sólo en listados globales excluye CUSTOM ajenos al
ensayo; todos los estándar y CUSTOM propios se comparan. Las respuestas crudas
se conservan. No se transforman errores, tipos, generation ni números.

Las mutaciones originales usan sólo recursos frescos, con GET 404 previo y
DELETE+GET 404 al limpiar. No se escriben tablas originales directamente.
Los PUT aggregates vivos son `[]`: crear un aggregate global deja una fila que
la API HTTP de Placement no permite eliminar. La asociación positiva, filtro
member_of con resultado y eliminación por cascade se probaron en MySQL aislado;
no se atribuyen a la comparación viva. Resources se prueba sin allocations:
B09 debe restar el uso antes de permitir reservas.

## Reproducción

1. Iniciar el laboratorio sólo durante la comparación. Preparar identidad de
   ensayo por Keystone, con roles por credencial y vencimiento; mantener tokens
   y credencial del validador en archivos privados fuera del repo.
2. Provisionar schemas Go vacíos separados y cuentas migrate/serve, siguiendo
   docs/placement/b07-development.md. Con opt-in PLACEMENT_GO_MYSQL_TEST=1 y
   PLACEMENT_GO_TEST_{MIGRATE,APP}_DSN_FILE, ejecutar cada suite en su DB nueva:
   `make placement-test-catalogs`, `make placement-test-binary`,
   `make placement-test-upgrade`. Ejecutar `make check`.
3. Compilar con `make placement-build`; ejecutar migrate explícito y serve con
   MySQL/Keystone reales, URL pública y mount path iguales. El ensayo usó un
   reverse proxy loopback temporal que sólo quita /placement de la solicitud,
   sin transformar respuestas; su presencia está en provenance.
4. Ejecutar, con destinos de evidencia nuevos y los tres token files privados:

   ```sh
   python3 scripts/placement_catalog_differential.py \
     --python-url http://192.168.122.10/placement --go-url "$GO_PLACEMENT_URL" \
     --admin-token-file "$ADMIN_TOKEN_FILE" --service-token-file "$SERVICE_TOKEN_FILE" \
     --reader-token-file "$READER_TOKEN_FILE" --output "$NEW_B08_OUTPUT" --allow-mutations
   python3 scripts/placement_differential.py \
     --python-url http://192.168.122.10/placement --go-url "$GO_PLACEMENT_URL" \
     --admin-token-file "$ADMIN_TOKEN_FILE" --service-token-file "$SERVICE_TOKEN_FILE" \
     --reader-token-file "$READER_TOKEN_FILE" --output "$NEW_B07_OUTPUT" --allow-mutations
   ```

5. Verificar reportes/cleanup y reinicio del ejecutable sobre la misma DB.
   Eliminar identidades de ensayo y contenedores propios, quitar archivos de
   secretos y apagar tt-openstack-lab. No reutilizar ni sobrescribir estas capturas.
