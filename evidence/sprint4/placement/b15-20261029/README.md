# B15 — suite integral real y repetible

Base B14 1a2efa7. Fecha local 2026-10-29. Helpers públicos en scripts/integration,
entrypoint placement_suite.py y target opt-in make placement-integration.
El contrato y reproducción están en docs/placement/b15-integration.md.

La aceptación usa las corridas finales run-06 y run-07 sobre la misma suite,
helpers y binario. Cada una exige once casos y ocho flags de limpieza. Sus
reportes, hashes, recursos públicos y snapshots se conservan por separado.
Incluyen un boot normal, dos boots concurrentes ACTIVE y un ERROR/NoValidHost
por falta de disco en cada corrida. Claims/CAS concurrentes, restart, auth y
rollback por timer usan datos/servicios reales; no son fixtures ni bypass.

Los ensayos prototype-01/02 pasaron antes de añadir retención ante cleanup fallido;
prototype-03 pasó con esa protección; prototype-04 pasó con exclusión del laboratorio.
prototype-05 falló con OSError durante preparación y no cuenta como PASS: sólo
terminó restauración, con cleanup completo. El fallo se localizó antes de crear
el runtime. La comprobación de puertos sin SO_REUSEADDR podía rechazar TIME_WAIT;
se reprodujo en sockets reales y se corrigió sin permitir un listener activo ni
usar SO_REUSEPORT. El diagnóstico posterior, ya liberados los puertos, no prueba
el errno del fallo histórico. Desde la corrección se registra errno para OSError.
Las dos corridas finales deben pasar consecutivamente, sin dormir para ocultar
ese problema de repetición. Fuentes intermedias archivadas como texto para no
contaminar los paquetes/runtime de CI.

Otras correcciones: un timer --collect ya vencido puede no existir cuando se
restaura de nuevo; eso es idempotente, sin silenciar fallos reales al detenerlo.
Si falla la limpieza de una VM, se retienen base/credenciales/ledger para recuperar;
una prueba roja/verdes impide desmontarlos prematuramente. La lease exclusiva
impide dos suites en paralelo. Tests también rechazan output existente, ausencia
de opt-in, proyectos/nombres ajenos y PASS con casos/cleanup incompletos.

Los reportes usan la identidad Nova descubierta en Keystone y conservan la
identidad/owner de cada reserva. Snapshot original antes/después incluye provider,
inventario, traits, aggregates, allocations, hash Apache y UUID MySQL. No se
normalizan cantidades, generaciones, estados, códigos o tipos para ocultar fallos.
Los schemas/servidores MySQL y user/project de Keystone son nuevos por corrida;
no se escribe SQL en Placement original ni se cambian Excel/S2/Nova.

Ningún archivo de evidencia contiene cnf, DSN, password, token, adminPass o backup
privado. El lock vacío de coordinación es un archivo persistente sin secretos;
las unidades, datos, perfiles e identidades temporales se retiran. El estado final
del lab y el apagado de VM se registran en lab-final-state.json y servicios finales.
B17/B18 y B26 no se acreditan por estas pruebas funcionales.

Mapa de fuentes históricas: placement_suite-v1.py.txt corresponde a prototype-01/02;
v2 a prototype-03; v3 a prototype-04/05. Los helpers runtime/preflight antes del
arreglo de puertos están en sus archivos before-port-fix.txt para esos cinco
ensayos. La revisión final usa source-files.sha256 y acceptance.json, que coteja
ambas corridas con el script actual y binario idéntico, en servidores MySQL distintos.
