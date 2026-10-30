# B13 — sustitución reversible de Placement

2026-10-29, cierre técnico del perfil del laboratorio. Base integrada B12 c2cadc7,
protocolo B16 d7c89e6. [Evidencia](../../evidence/sprint4/placement/b13-20261029/README.md).
B13 verifica publicación del compute y cliente Nova; B14 exige el boot real.

## Resultado comprobado

El laboratorio tenía un compute/provider plano, sin sharing ni allocations y sin
instancias. Se ejecutó Go en la misma VM, sobre MySQL 8.0.46 propio en 127.0.0.1:13316,
datadir, socket, UUID de servidor, schema y cuentas distintos del original. Las
seis migraciones existentes no se modificaron. Migrador con DDL, runtime sólo DML;
serve comprueba sus grants e historial. No se reutiliza el backend de DevStack.

Placement Python corre en devstack@placement-api.service (uWSGI). Apache publica
/placement. Se guardaron bytes/hash/modo/owner de su site en un directorio privado;
se cambió exclusivamente esa ruta al listener Go 127.0.0.1:18779 y se detuvo la
unidad original. Keystone/Glance y catálogo/configuración de Nova se conservaron.
Reinicio controlado de nova-compute/scheduler refrescó sus caches.

Nova real creó en Go el provider de UUID 3b7cc290-1329-47d2-9502-890059c82d1b,
publicó sus tres inventarios y traits. Los logs Go atribuyen esas peticiones a
la identidad Nova validada por Keystone (c40c4337691f4a94a519104114abb32f,
proyecto service). No se toman IDs/roles de cabeceras aportadas por el cliente.
Pruebas cubren spoofing, ausencia de identidad en discovery/401 y redacción.

El cliente desplegado nova.scheduler.client.report, su RequestSpec/ResourceRequest
y sus credenciales nova.conf obtuvieron candidates 1.36 del provider real con
VCPU:1/MEMORY_MB:512/DISK_GB:1 y root_required OWNER_NOVA,!COMPUTE_STATUS_DISABLED.
Es un probe con la biblioteca Nova sin cambiar su código, separado del proceso
scheduler; no se presenta como boot. B14 debe mostrar server create→ACTIVE.

Inventarios/constraints, traits, aggregates, identidad/root del provider y
allocations coinciden lógicamente. La generación original era 3 y Go reconstruyó
una historia nueva con 2; ambos valores se conservan y no se declaran iguales ni
se normalizan como un pase diferencial. Sólo se usa la DB vacía porque no había
cargas/reservas preexistentes. Un laboratorio con VMs necesita migración/aislamiento.

Se restauró el site byte a byte (SHA 07fab01f3c097319f9ab9d86bb22cf8fc0432381bb4c50e5fd58948c3b29f65f),
uWSGI quedó activo y la URL volvió a anunciar 1.39. Provider/inventarios/traits y
allocations originales se consultaron después de restaurar. Go anunciaba 1.36
mientras Python estaba detenido. La restauración también pasó en el intento que
falló por construcción del objeto del probe, antes de corregir ese helper.

## Herramientas y operación

- scripts/integration/lab_identity.py: prepara/refresca/retira sólo user/project
  tt-placement propios, app creds por rol y archivos 0600 en root 0700. Usa SDK
  instalado dentro de la VM y openrc admin; no imprime secretos.
- placement_preflight.py: consultas HTTP y SQL sólo lectura del original, versiones,
  topología, reservas, políticas, cgroup/PIDs y hash Apache. No sigue redirects con
  credenciales. Las listas de servidores/imagen/flavor/red se inspeccionan aparte.
- placement_runtime.py: crea una instancia MySQL nativa privada y unidades systemd
  propias. Bootstrap inicial por socket sin TCP mientras root no tiene password.
  Luego configura cuenta root privada y migrador/app limitados. El backend exige
  un perfil AppArmor nuevo acotado a su root; no modifica/desactiva el perfil de
  mysqld original. Incluye restart/replace de Go y cleanup de unidades/datos/perfil.
- placement_switch.py: activate/restore con hash previo como lease, backup privado,
  reemplazo atómico del site, configtest y reload Apache. Captura/restaura si
  proxy_http estaba habilitado. Exige preflight/Go sanos y corta ante cambios ajenos.
  Activa un timer de recuperación automático a 600s por defecto, cancelado sólo
  tras verificar rollback. Nunca desmonta Go/MySQL mientras la ruta sigue en Go.
- nova_candidates_probe.py: usa la biblioteca real de Nova con su configuración;
  no abre DB/RPC ni escribe allocations. Conserva query, versión y candidates.

Los comandos son opt-in cuando mutan; requieren paths nuevos privados y puertos
libres. Preparar la VM existente, compilar/copiar el binario e inspeccionar primero
el manifest de recursos. Ejemplo del cambio tras preparar runtime:

```sh
python placement_switch.py activate --root "$OWN_PRIVATE_RUNTIME" \
  --expected-original-sha "$VERIFIED_APACHE_SHA" --allow-mutations
# Publicar recursos con el compute real y ejecutar el cliente Nova.
python placement_switch.py restore --root "$OWN_PRIVATE_RUNTIME" --allow-mutations
```

Los helpers remotos necesitan las librerías ya instaladas en DevStack y sudo para
sus unidades, site y perfil propio. No inician DevStack en el host ni reprovisionan
la VM. El runner integral opt-in y las inyecciones/cleanup automatizados son B15.

## Fallos observados y límites

Los dos primeros arranques MySQL fueron rechazados por AppArmor: el primero usaba
el perfil original sin permiso para el root nuevo; el segundo perfil propio no
permitía /tmp. Se mantuvo el perfil original y se añadió tmpdir dentro del root
propio. Cada intento fallido se limpió antes de otro prepare; se conserva su error.
Una comprobación CLI con --defaults-extra-file heredó ~/.my.cnf y consultó el
servidor original, sólo con SELECT. Se detectó por UUID: el probe correcto usa
--defaults-file y protocolo SOCKET. Bootstrap usa socket explícito PyMySQL y Go
usa DSN explícito 13316; sus escrituras siempre fueron al servidor independiente.
UUIDs distintos y ruta/puerto reales están registrados; se corrigió readiness.

Los errores iniciales del probe afectaban a la construcción de RequestGroup/
RequestSpec del helper; no al servicio Go. Se probó el probe corregido contra
Python antes de repetir el ciclo. make check y guards locales pasaron. Las pruebas
rojas y correcciones del logging incluyen discovery sin identidad autenticada.

No se acredita HA, nested/sharing, políticas completas ni llamadas fuera de los
perfiles B10/B12. No se cambió Excel/S2. Al cerrar B13, el original estaba restaurado;
el runtime/identidad/VM siguen en uso para B14 y su limpieza final se registra allí.

La [aceptación B14](b14-acceptance.md) registra el cierre posterior: boot ACTIVE,
limpieza de runtime/identidad/perfil propios y VM apagada, con original restaurado.
