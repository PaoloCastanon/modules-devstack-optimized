# Campaña conjunta por API joint-api-v1 — oficial

2026-11-28, 00:41–01:50 UTC. Código `2410330`, binarios `placement`
`91866d8f…` y `glance` `652a7337…`, VM `tt-openstack-lab` (4 vCPU, 8 GiB).
[Protocolo](../../../../docs/benchmarks/joint-api-protocol-v1.json) ·
[runbook](../../../../docs/benchmarks/joint-api-campaign.md).

**Resultado: 72/72 ventanas válidas, limpieza completa, análisis aceptado.**
Primer intento oficial; sin ventanas descartadas. Tablas completas:
[summary.md](summary.md). Datos por corrida, por operación y controles:
`summary.json`. Las muestras crudas (54 MiB) quedan fuera de Git, en
`reports/campaigns/joint-api-official-20261128/` del equipo que corrió la campaña.

## Lectura (medianas de 3 repeticiones)

**Placement acompañado de Glance**, Go frente a Python:
- Rendimiento: entre 2,9× y 15× más ops/s según el par y C.
- CPU por petición: 0,06–0,10×.
- PSS: ~15–18 MiB frente a ~181–195 MiB (0,08–0,09×).
- Con los dos servicios en Go (go-go frente a python-python), Placement hace
  5,8–9× más ops/s y su p99 baja (p. ej. `nova` C=16: 421 → 108 ms).

**Glance acompañado de Placement**, Go frente a Python:
- CPU por petición: 0,05–0,17×.
- PSS: ~17–20 MiB frente a ~234–242 MiB (0,07–0,08×).
- Rendimiento: 1,4–7,9× más ops/s.

**Pila completa (los dos servicios activos):** PSS de Placement+Glance
~32–37 MiB en go-go frente a ~416–433 MiB en python-python (≈ 0,08×); CPU de
los dos servicios 0,29–0,42× para el mismo intervalo.

**Competencia por la CPU (efecto del lazo cerrado):** el servicio acompañante
más rápido genera más carga. Con Glance Go, Placement Python baja (`nova` C=16:
88 → 33 ops/s) y su CPU/op sube 1,5×. Con Placement Go, Glance Python baja
(`nova` C=16: 131 → 57 ops/s). En go-go los dos comparten la CPU y cada uno
queda por debajo de su variante con el acompañante en Python, pero muy por
encima de python-python. La CPU ocupada de la VM llega al 82–99 % con C=16.

## Límites

- 3 repeticiones por celda: mediana y rango, sin significación estadística.
- Lazo cerrado en ambos generadores (host). La condición "acompañado" mide la
  competencia real por los 4 vCPU, no la de una carga de fondo fija.
- Despliegues distintos (workers, cachés, pools): no se atribuye todo al lenguaje.
- PSS muestreado a 1 Hz; el pico es el observado.
