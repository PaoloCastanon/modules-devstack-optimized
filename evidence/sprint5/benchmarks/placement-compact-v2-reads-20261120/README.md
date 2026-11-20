# Placement compact-v2: celdas de lectura tras caché de tokens y pool — 2026-11-20

Protocolo `placement-compact-v2-reads` ([definición](../../../../docs/benchmarks/placement-protocol-compact-v2-reads.json)):
provider, inventories y allocations-read; concurrencias 1/16; tres pares
Python/Go por celda; 10 s de warm-up y 30 s medidos. **36/36 ventanas válidas**,
0 errores, B17 y B18 aceptados por el análisis. Mismos instrumentos y VM que
compact-v2; solo cambia el binario Go (commit `9a5c18e`, caché de tokens 300 s
y pool de 16 conexiones por defecto).

- `pilot/`: 12 ventanas cortas (1 s + 4 s), 12/12 válidas. No se agregan.
- `official/`: manifest, muestras por request y recursos CPU/PSS por ventana.
- `summary.json`: análisis de `analyze_placement.py`.
- `publication/`: tablas y figuras de `publish_placement_results.py`.
- `go-build.txt`: `go version -m` del binario medido
  (SHA-256 `8bf268cbc8a8e71e74211002d60852ceef15f4008c1c6ad705dda4bcb390a48b`).

Cleanup: original sin cambios, runtime, identidad y raíz privada propios
retirados, controles restaurados. Después: Placement original activo, un solo
provider, cero instancias. Timers apt reactivados y VM apagada.

No reemplaza ni se mezcla con la campaña compact-v2 del 2026-10-31.
