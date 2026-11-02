# B15 — suite automática de integración Placement

Fecha local 2026-10-29. Base B14 1a2efa7. Criterio: dos corridas completas
consecutivas sobre la misma revisión de la suite, con recursos privados nuevos,
original restaurado y limpieza comprobada. **B15 completado técnicamente:** las
corridas finales 06/07 pasan 11/11 casos cada una y todos los flags de cleanup.
La ejecución usa Nova, Keystone,
Glance original, MySQL y Placement Go reales dentro de tt-openstack-lab.
[Resultados y fuentes](../../evidence/sprint4/placement/b15-20261029/README.md).
No cambia Excel/S2 ni acredita benchmarks B17/B18 o Glance Go/B26.

## Comando y prerrequisitos

El entrypoint público es scripts/integration/placement_suite.py. Se ejecuta
**dentro de la VM existente**, con las librerías de DevStack y sudo operativo.
No inicia/provisiona la VM y no ejecuta DevStack en el host. Antes de usarlo:
compilar/copiar una revisión conocida del binario Go y estos helpers; inspeccionar
los IDs de provider, imagen ACTIVE, flavor no-BFV y red existentes. El laboratorio
debe tener exactamente un provider plano, sin sharing, reservas ni instancias.
No modifica el inventario para generar escasez ni borra recursos preexistentes.

```sh
source /opt/stack/data/venv/bin/activate
set +u
source /opt/stack/devstack/openrc admin admin
make placement-integration PLACEMENT_INTEGRATION_ARGS="\
  --binary $KNOWN_PLACEMENT_BINARY \
  --provider-id $VERIFIED_PROVIDER_UUID \
  --image-id $VERIFIED_IMAGE_UUID --flavor-id $VERIFIED_FLAVOR_ID \
  --network-id $VERIFIED_NETWORK_UUID \
  --work-root $NEW_ABSOLUTE_PRIVATE_ROOT --output $NEW_OUTPUT \
  --allow-mutations"
```

También se puede ejecutar el script directamente con el Python del venv.
El root privado debe ser nuevo, absoluto/canónico y de nombre tt-placement-<id>;
su padre debe existir. Output también debe ser nuevo. El script no recibe secretos
por argumento: usa admin openrc y genera credenciales de aplicación propias en
archivos privados. No copiar openrc, DSN, cnf, backups Apache o tokens al repo.

Una lease exclusiva no bloqueante en /run/lock/tt-placement-integration.lock evita
dos coordinadores de la suite simultáneos. Se comprueban owner, permisos y tipo,
sin seguir symlinks. El inode vacío se conserva entre corridas para no crear una
carrera por unlink; no contiene secretos y no es un servicio activo. Usar un
laboratorio exclusivo también respecto de cambios manuales u otras herramientas.

## Preparación automática

1. Comprobar ausencia de instancias por API all-projects e IDs explícitos de
   imagen/flavor/red. Crear user/project tt-placement y app creds limitadas por rol.
2. Capturar preflight original mediante HTTP y SQL sólo lectura: provider,
   inventarios, traits, aggregates, allocations, versiones, política, PIDs y hash
   del site Apache. Descubrir la identidad Nova por Keystone, sin hardcodear su ID.
3. Crear MySQL nativo en datadir/socket/puerto propios, schema y cuentas migrador/
   runtime nuevos, perfil AppArmor independiente y Go bajo unidades systemd propias.
   Verificar el UUID de servidor/esquema/puerto a través del DSN app explícito:
   no puede ser el servidor original. Serve comprueba grants e historial de migración.
4. Activar la ruta Go con backup/hash y lease de rollback acotada. Detener sólo la
   unidad Placement original y reiniciar compute/scheduler para refrescar caches.
   Keystone y Glance original siguen operativos; no se cambia Nova ni su catálogo.

## Once casos obligatorios

| Caso | Aserción observable |
|---|---|
| preflight_and_isolation | Lab vacío, IDs correctos, provider plano y MySQL propio con UUID distinto |
| tracker | Nova publica el provider real con inventarios/constraints, traits y aggregates equivalentes; cliente Nova obtiene candidates |
| auth_fail_closed | Service acepta claim; reader PUT/DELETE, cabeceras falsas, token inválido/ausente se rechazan sin cambiar estado |
| restart_persistence | Nuevo PID Go conserva provider/reserva/owner/generaciones; siguiente consulta del cliente Nova funciona |
| consumer_cas | Dos PUT simultáneos del mismo consumer nuevo: un 204, un 409 placement.concurrent_update y una reserva |
| capacity_concurrency | Cinco claims VCPU4 con capacidad16: cuatro 204, un 409, uso16 sin sobreasignación y rollback del perdedor |
| nova_no_capacity | Flavor temporal con disco mayor que la capacidad: candidates vacíos, Nova ERROR/NoValidHost, sin allocations; server/flavor retirados |
| nova_boot_and_release | B14: ACTIVE, recursos/owner/generación correctos, delete y usos devueltos al baseline |
| nova_concurrent_boots | Dos creaciones reales concurrentes llegan a ACTIVE; allocations individuales y usos sumados correctos; ambas se borran y liberan |
| failure_rollback | Excepción deliberada con un claim propio: timer restaura Python sin restore manual; claim retirado por loopback Go |
| original_restoration | Original sano, mismo estado/hash/UUID MySQL y siguiente consulta real del cliente Nova aprobada |

La carrera de capacidad se deriva del inventario VCPU y tiene máximo de ocho
participantes; no asume que un request puede superar max_unit. Los valores de la
tabla son los observados en este lab. CAS y capacidad se prueban por HTTP de forma
determinista; el caso de boots concurrentes prueba además el flujo Nova real.
Las generaciones originales/Go tienen historias independientes: se conservan,
no se normalizan para declarar igualdad. Reinicio/denegaciones sí exigen que sus
snapshots permanezcan iguales cuando no corresponde una mutación.

## Limpieza, fallos y evidencia

Cada consumidor, flavor y server tiene ID/nombre propio registrado; el ledger
privado se guarda antes de crear recursos y se actualiza durante los creates.
Antes de borrar una VM se verifica nombre exacto y proyecto. Un create incierto
sólo se resuelve por ese nombre y se rechaza ambigüedad. No se serializa adminPass
ni el Server entero. HTTP con credenciales no sigue redirects; el comparador
suprime evidencia si una respuesta refleja un token real. Los logs Go contienen
IDs validados por Keystone, sin tokens/cabeceras falsas.

Finally elimina los servers/flavors/claims propios, verifica ausencia por API,
restaura configuración y proceso Python, repite preflight/cliente Nova y elimina
unidades, datadir/cuentas/perfil de MySQL, Go, identidad/app creds y root privado.
Ocho flags de cleanup deben ser true. Si una VM/reserva o rollback no se puede
limpiar, se conserva backend, validador y ledger para recuperación: no se destruye
el estado ni se transforma el fallo en PASS. Se guarda sólo el tipo/errno del error,
sin mensajes de excepciones que puedan incluir secretos.

El caso de fallo controlado se ejecuta sin VMs pendientes. Verifica el timer real
con una reserva sintética propia; no mata servicios ajenos. El timer restaura la
ruta, no promete limpiar VMs ante SIGKILL del coordinador ni HA. Un corte abrupto
requiere revisar el ledger privado y completar limpieza antes de otra sesión.
La lease del cambio de ruta sigue verificando hashes para no sobrescribir cambios
ajenos. Restaurar otra vez debe ser idempotente aunque el timer ya fue recolectado.

Report.json conserva casos, checkouts/hashes, IDs públicos, tiempos y cleanup;
los directorios de casos contienen snapshots, respuestas CAS/capacidad, boots,
queries y logs Go saneados. Un caso omitido o un cleanup fallido produce exit no
cero. Las suites en vivo están fuera de make check y del CI ordinario; CI ejecuta
sus guards/tests locales y las regresiones existentes. Se conservan ensayos
intermedios y fallidos sin mezclarlos con las dos corridas de aceptación.

Después de la sesión comprobar originales activos, ausencia de unidades/perfiles
propios y de instancias, y apagar tt-openstack-lab. B17/B18 requieren campañas de
medición separadas siguiendo B16; tiempos de esta suite no son benchmarks.
