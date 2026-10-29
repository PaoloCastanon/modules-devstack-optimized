# B11 — cierre de persistencia del perfil Placement

Fecha: 2026-10-29. Base B10 b42c273. **B11 completado para el esquema propio y
las rutas priorizadas.** El cierre integra las pruebas previas B07–B10 con
recuperación de MySQL, caída del proceso y backup/restore completos.

La [campaña B11](../../evidence/sprint3/placement/b11-close-20261029/README.md)
usa MySQL 8.0.46 con volumen Docker propio, schemas/accounts distintos y Keystone
real. El servicio ejecutado es el binario normal, con su cuenta DML y sin bypass.
Nueve verificaciones obligatorias pasaron:

1. Una cuenta con DDL no puede ejecutar serve.
2. La cuenta runtime no puede crear tablas.
3. La cuenta runtime no puede leer el schema sentinel separado.
4. SIGKILL de Go conserva el estado al iniciar otro proceso.
5. SIGKILL de MySQL conserva todas las filas confirmadas, incluidos timestamps.
6. Una actualización de owner en transacción abierta, confirmada sólo dentro de
   su conexión antes del kill, se revierte mediante recuperación InnoDB.
7. Un backup se restaura en otro schema/cuenta y conserva todas las filas y HTTP.
8. El sentinel no se modifica.
9. Las reservas/restos restaurados se limpian y el contenedor/volumen se eliminan.

Se comparan provider, inventario, traits, aggregates, consumer, allocations por
provider, usages y candidates. Un dump ordenado de todas las tablas del esquema
se compara byte a byte antes/después y tras restore; el hash incluye versiones/
checksums de migración y timestamps. Sólo se normalizan campos HTTP temporales
permitidos, request_id/origen/Vary; no los datos SQL ni generaciones/owner.
La restauración desactiva FK sólo en la sesión administrativa de replay para
recrear tablas en orden de dump; el runtime mantiene todas las restricciones.
Igualdad completa de filas y estado HTTP verifica el resultado restaurado.

## Evidencia transversal

B07 prueba rollback de inventories y conflictos; B08 traits/classes y upgrades;
B09 reemplazos de allocations en varios providers, CAS del consumer, SQL fallido
final y carreras; B10 snapshots RR concurrentes y uso persistido. Todas sus suites
MySQL vuelven a pasar en el cierre B10 con la migración v6. v1–v5 permanecen intactas.
Ver logs en la evidencia B10 y make check para vet/race/build de ambos dominios.
Las cuentas sólo tienen grants directos del schema propio; roles/global grants,
cuentas/nombres originales y opciones MySQL inseguras se rechazan al arrancar.

El primer intento del helper no llegó a cerrar la recuperación tras reiniciar el
contenedor con binding aleatorio. Se corrigió el helper para conservar un puerto
explícito y usar create/start con ownership registrado; el segundo pasó las nueve
aserciones. Ambos informes se conservan; el primero no cuenta como PASS.
No se modificó el servicio productivo para inyectar fallos o recuperar datos.

## Operación y límites

scripts/placement_persistence_campaign.py requiere --allow-mutations, un --output
nuevo, --keystone-url real, --validator-file y --admin-token-file privados. Crea
identificadores aleatorios propios; sólo elimina contenedor/volumen cuyo create
confirmó. No conecta a la DB original. La cuenta de migración ejecuta migrate
explícito; serve exige historial completo y limpio. Si DDL falla no se arregla
historial dirty automáticamente. Para rollback binario se restaura el backup
previo completo; no hay down migrations automáticas.

Este ensayo verifica durabilidad y restauración funcional de datos sintéticos,
no HA, réplica, pérdidas de host/disco ni rendimiento. No inicia DevStack, cambia
endpoints, modifica Excel/S2 o acredita Nova con Go. La VM se usa para Keystone
durante B12 y se apaga al cierre final; contenedores, volúmenes y secretos de B11
fueron eliminados. B12 verifica autenticación; B13–B15 conservan la integración.
