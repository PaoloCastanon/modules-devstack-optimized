# B14 — instancia ACTIVE, reserva y liberación verificadas

Fecha local 2026-10-29. Base B13 55b0019. PASS real de Nova/Placement Go/Keystone,
Glance original y MySQL Go independiente dentro de la VM. No es fixture ni probe.

live/boot/report.json registra server 1af1c553-3927-431a-be9b-d7a1f3010006,
BUILD/ACTIVE, owner, allocations, generación y usages antes/durante/después.
Todos los checks de boot/allocations/delete/release/usages pasan, sin execution_error
ni cleanup_error. VCPU1/MEMORY_MB512/DISK_GB1, consumer gen1, provider gen3.
live/go-requests.json atribuye candidates, PUT/DELETE allocations a Nova validada;
live/activation.json acredita original detenido y ruta Go con lease de 900s.
El SDK no guarda adminPass ni objetos Server completos.

live/restoration.log vuelve a Python 1.39/site SHA exacto. El cliente Nova real
obtiene candidates del original después (original-nova-client-after.json).
original-final-preflight.json, final-before-cleanup.txt y lab-final-state.json
verifican original intacto, ausencia de VMs, imagen original ACTIVE y VM apagada.
services-final.txt registra los cinco servicios originales activos antes del
shutdown, ausencia de unidades/perfiles tt-placement y SHA del perfil original.
runtime-cleanup.json e identity-cleanup.json acreditan retirada de Go/MySQL,
datadir/cuentas/DSN/perfil propios y user/project/app creds/directorio privado.
No se guardan tokens, secretos, cnf o backups privados en esta evidencia.

make-check.log PASS (ambos dominios: vet/race/build, harness/infra); guards.log
comprueba opt-in antes de SDK/mutaciones y output existente antes de borrar.
source-files.sha256 identifica el helper y logging; el binario está identificado
en la evidencia B13. reproduction/host-cycle.py.txt conserva el orquestador exacto
con rutas/IDs de esta sesión. Para repetir usar un runtime/identidad nuevos y los
helpers públicos B13/B14; no reutilizar IDs o roots históricos.

El ensayo es uno completo positivo. B15 requiere suite pública con dos corridas,
negativos de capacidad/CAS/auth/restart y fallo/rollback. B17/B18 no se ejecutaron.
Glance Go/B26, API Placement completa y HA permanecen fuera. Excel y S2 intactos.
