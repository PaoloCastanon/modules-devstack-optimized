# Placement compact-v2: candidates, claim-cycle y nova-mix tras caché y pool — 2026-11-20

Protocolo `placement-compact-v2-nova` ([definición](../../../../docs/benchmarks/placement-protocol-compact-v2-nova.json)):
candidates, claim-cycle y nova-mix; concurrencias 1/16; tres pares Python/Go
por celda; 10 s de warm-up y 30 s medidos. **36/36 ventanas válidas**, 0 errores,
B17 y B18 aceptados por el análisis. Mismo binario que la re-medición de lecturas
(SHA-256 `8bf268cbc8a8e71e74211002d60852ceef15f4008c1c6ad705dda4bcb390a48b`).

- `pilot/`: 12 ventanas cortas (1 s + 4 s), 12/12 válidas. No se agregan.
- `official/`, `summary.json`, `publication/` y `go-build.txt`: como en
  [la re-medición de lecturas](../placement-compact-v2-reads-20261120/README.md).

Python rindió 20–30 % menos que en compact-v2 (31-oct) en cuatro de las seis
celdas, de forma estable dentro de la campaña (sin deriva entre repeticiones).
La causa no se identificó. Por eso el informe compara también Go contra Go.

Cleanup: original sin cambios, runtime, identidad y raíz privada propios
retirados, controles restaurados. Después: Placement original activo, un
provider, cero instancias. Timers apt reactivados y VM apagada.
