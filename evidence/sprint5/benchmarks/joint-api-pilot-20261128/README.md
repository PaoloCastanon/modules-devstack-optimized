# Piloto joint-api-v1 (Placement y Glance cargados a la vez)

2026-11-28. Código `7ce3cb8`, binarios `placement` `91866d8f…` y `glance`
`652a7337…`. VM `tt-openstack-lab` (4 vCPU, 8 GiB). Piloto: 2 s de
calentamiento + 5 s de medida, 1 repetición, **24/24 ventanas válidas, limpieza
completa**; no es resultado oficial. Tablas: [summary.md](summary.md).

Intento 1 (no usado): se detuvo en la ventana 5 por un solape del 89 %. Con 16
clientes, la limpieza del calentamiento de Placement tardaba ~0,7 s más que la
de Glance. Además, la limpieza final restauraba los registros de controles en
el orden equivocado; el estado de la VM era correcto y se terminó a mano. Las
dos cosas se corrigieron en `7ce3cb8` (barrera común antes de medir y orden
inverso de restauración).

Observación para el análisis: con carga en lazo cerrado, el servicio
acompañante más rápido genera más carga y consume más CPU de la VM (p. ej.
Glance Go con 16 clientes reduce el rendimiento de Placement Python). La
condición "acompañado" mide esa competencia real por la CPU.
