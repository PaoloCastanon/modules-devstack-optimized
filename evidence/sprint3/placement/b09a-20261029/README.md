# B09-A — allocations atómicas, 2026-10-29

**Entrega A validada; B09 continúa en progreso.** El ejecutable conserva default
1.0 y ahora negocia hasta 1.8 para las rutas seleccionadas. El formato de escritura
1.12 y consumer generation 1.28, requeridos por Nova, todavía están pendientes.
No se acredita candidates, sustitución Nova ni toda la API del rango anunciado.

## Resultados observados

- `live-2/b09-run/report.json`: **60/60** status, cabeceras estables, cuerpos y
  estados coinciden con Placement Python. Requests, respuestas crudas, antes/
  después, status/generación esperados y cleanup están conservados.
- `b07-run/`: **57/57** respuestas/status/estados en 1.0, runner B07 intacto.
- `b08-run/`: **82/82**, runner B08 intacto, incluidas sus normalizaciones de
  catálogos documentadas en B08. No se flexibilizó ninguno de los comparadores.
- `sql-3/allocations.log`: MySQL 8.0.46 real, cuentas migrate/serve distintas,
  schema propio nuevo; upgrade v3→v4, preservación de inventario/traits B08,
  sustitución descontando la reserva previa, rollback entre providers y ante
  error SQL final, rollback de propietario de consumer, carreras de capacidad
  (uno y varios providers), reemplazos concurrentes del mismo consumer, pool
  nuevo, clase CUSTOM asignada que se renombra por cascade y GC de consumers.
- `sql-3/{catalog,b07,binary,collision}.log`: regresiones MySQL B07/B08,
  ejecutable con migraciones y guard de upgrade con colisiones; cada suite usa
  un schema vacío diferente. Las migraciones v1–v3 mantienen sus hashes.
- `live-2/restart-proof.json`: SIGTERM exit 0 y nuevo proceso con la misma DB;
  provider/inventario, reservas, uso y generaciones se conservan. No se reinició
  el servidor MySQL ni se simuló crash abrupto.
- `make-check.log`: gate local completo con formato, vet, race y compilación de
  ambos dominios; harness/documentación, scripts e infraestructura sin iniciar VM.

La prueba HTTP incluye 1.8 con propietario, consultas y DELETE en 1.0, reader
403, falta de token 401 y service autorizado; sobrecapacidad, constraints min/
max/step, capacidad fraccionaria, sustitución entre providers, uso en filtros,
protección de inventario/provider y repetición de operaciones. Las escrituras
positivas 1.0–1.7 tienen pruebas locales de protocolo y de la transacción; no se
atribuyen a esta comparación viva. Los nueve errores de esquema capturados
se reproducen literalmente desde tests/compatibility/fixtures/placement-b09-errors.json.

## Procedencia, decisiones y límites

Base: B08 6587805d1c53df81034292e36ecdbf3869c9198d; PR #7 abierto al iniciar.
Original: f4a89d3803cb23df22b8714569889ab39c918c34. Fuentes fijadas:
placement/{handlers,objects,schemas}/allocation.py, handlers/inventory.py,
objects/resource_provider.py, handlers/usage.py, policies/allocation.py y
handlers/util.py. `upstream-source.sha256` conserva hashes de las fuentes
inspeccionadas; `source-files.sha256` identifica las fuentes Go e inputs embebidos.
Cada live/provenance.json registra hash del ejecutable, toolchain y entorno.

Se corrigió una suposición del plan: upstream permite reducir inventario por
debajo del uso, conservando reservas. Un reemplazo igual o que mejora esa
sobrecapacidad se permite; una reserva que la empeora devuelve 409. Borrar el
inventario/provider en uso está bloqueado. DELETE allocations no incrementa
provider generation; PUT sí aumenta una vez cada provider solicitado, incluso
sin cambiar la reserva. Los providers que sólo pierden una reserva no aumentan.

`original-preflight.json` mostró que faltaban los registros globales auxiliares
para consumers anteriores a 1.8. Una escritura legacy habría creado filas que
la API no permite limpiar. Por eso se incorporó el cambio puntual de 1.8 y el
ensayo usó project/user ya existentes en Placement. No basta con que existan en
Keystone. Los IDs de la identidad temporal del ensayo son otros: las políticas
admin/service de Placement permiten administrar estas reservas globales.
`original-cleanup.json` confirma que los IDs globales no cambiaron, no aparecieron
registros incomplete y no quedaron filas de providers, consumers o allocations
del ensayo. La consulta SQL al original fue exclusivamente de lectura.

La comparación B09 sólo normaliza lo ya permitido en B07: request_id, origen
configurado en Location y orden/case de tokens Vary. No se transforman errores,
tipos, valores numéricos, generaciones ni identidades de consumer/provider.

`api-red.log` conserva el 404 inicial de la ruta antes de implementarla.
`live-1/` obtuvo 52/52 antes de añadir los casos de capacidad fraccionaria y
constraints; `live-2/` amplía a 60/60. SQL 1 probó el núcleo inicial sin owner;
SQL 2 añade owner 1.8; SQL 3 añade fallo final de consumer y carrera entre varios
providers. Las ampliaciones se ejecutaron en DB nueva. La comparación final
incluye además la corrección de precisión int/float en capacidades grandes.

Todos los tokens y credenciales se mantuvieron en archivos privados externos.
`identity-cleanup.json` confirma la eliminación de usuario/proyecto temporales;
`lab-final-state.json` confirma VM apagada. Los procesos/contenedores/datos
locales y archivos secretos se eliminaron al finalizar. No se cambiaron endpoints,
Nova, Excel ni evidencia S2.

## Reproducción

1. Preparar MySQL 8.0.46 descartable y cuentas propias separadas con los permisos
   de docs/placement/b07-development.md. Exportar PLACEMENT_GO_MYSQL_TEST=1 y
   PLACEMENT_GO_TEST_MIGRATE_DSN_FILE / PLACEMENT_GO_TEST_APP_DSN_FILE privados.
   Ejecutar cada suite en una DB nueva: `make placement-test-allocations`,
   `make placement-test-catalogs`, `make placement-test-mysql`,
   `make placement-test-binary`, `make placement-test-upgrade`. `make check` es local.
2. Iniciar el laboratorio sólo para la comparación. Preparar por Keystone roles
   admin/service/reader en credenciales de ensayo con vencimiento; archivos de
   tokens privados y credencial service del validador, sin secretos en el repo.
   Verificar por lectura que los project/user elegidos ya existen en Placement.
3. Compilar y ejecutar migrate explícito y serve con su cuenta DML, MySQL propio,
   Keystone real y URL pública /placement. El ensayo usó un reverse proxy loopback
   que sólo quita el prefijo de la solicitud; las respuestas permanecen intactas.
4. Ejecutar con una carpeta nueva:

   ```sh
   python3 scripts/placement_allocation_differential.py \
     --python-url http://192.168.122.10/placement --go-url "$GO_PLACEMENT_URL" \
     --project-id "$EXISTING_PLACEMENT_PROJECT_ID" --user-id "$EXISTING_PLACEMENT_USER_ID" \
     --admin-token-file "$ADMIN_TOKEN_FILE" --service-token-file "$SERVICE_TOKEN_FILE" \
     --reader-token-file "$READER_TOKEN_FILE" --output "$NEW_B09_OUTPUT" --allow-mutations
   ```

   Ejecutar también los runners B07/B08 con los mismos destinos/tokens y otras
   carpetas nuevas. No sobrescribir estas capturas ni las de Sprint 2.
5. Verificar restart con reservas, cleanup HTTP y lectura posterior de filas del
   original; eliminar identidad de ensayo y archivos privados, descartar procesos/
   MySQL propios y apagar tt-openstack-lab.
