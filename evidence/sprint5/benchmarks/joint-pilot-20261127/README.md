# Placement Go + Glance Go con Nova: E2E y piloto de la campaña conjunta

Fecha local 2026-11-27. `main` en `627691c` más la rama
`fix/glance-shared-db-bootstrap`. Binarios estáticos (`CGO_ENABLED=0`, Go
1.27.4): `glance` sha256 `652a7337…`, `placement` sha256 `91866d8f…`. VM
`tt-openstack-lab` (4 vCPU, 8 GiB), flavor `m1.nano`, red `private`, CirrOS
0.6.3 qcow2.

## E2E conjunto (`glance_suite.py --with-placement-go`)

| Corrida | Casos | Limpieza | Nova POST→ACTIVE |
|---|---|---|---|
| `e2e-run3` | 16/16 | completa | 8,4 s (caso) |
| `e2e-run4` | 16/16 | completa | 7,6 s (caso) |

Dos intentos previos fallaron antes de cambiar nada (originales intactos):
`python3` del sistema sin `keystoneauth1` (hay que usar
`/opt/stack/data/venv/bin/python3`) y la base `glance_go_shared` inexistente
en esta VM (Glance Go `migrate` reintentaba hasta rendirse). La corrige
`placement_runtime.ensure_shared_database`.

## Piloto joint-compact-v3

[Protocolo](../../../../docs/benchmarks/joint-protocol-compact-v3.json),
[runbook](../../../../docs/benchmarks/joint-campaign.md). **8/8 bloques
válidos, 12 boots, limpieza completa** (`report.json`). Tabla:
[analysis.md](analysis.md).

El intento 1 se detuvo en el bloque 2 (`NotFoundException` al esperar el borrado
de un servidor que ya no existía: defecto del runner, corregido); su limpieza
fue completa y sus datos no se usan.

## Límites

- Piloto: una repetición, no es resultado. Con n=1 por celda las diferencias
  de 1 s están dentro de la resolución del polling (1 s).
- Quedan en `_base` de Nova dos entradas de las corridas E2E y dos del intento 1
  del piloto; Nova las elimina con su limpieza periódica de imágenes sin uso.
