# Placement Go: caché de tokens y pool de MySQL

2026-11-20. Rama `feat/placement-token-cache-pool`. Motivo: en la campaña
compact-v2 (B17), Go quedó por debajo de Python en las lecturas simples
(provider y allocations-read, ~0,61–0,67× en throughput). Dos causas probables
estaban en la configuración, no en el lenguaje:

1. **Validación de tokens.** Go validaba cada token contra Keystone en cada
   petición. Placement oficial usa keystonemiddleware con
   `token_cache_time = 300` s y reutiliza la validación. Glance Go ya tenía ese
   caché (`internal/auth/token_cache.go`), pero Placement no lo activaba.
2. **Pool de MySQL.** El pool estaba fijo en 4 conexiones abiertas y 2
   inactivas. Con 16 clientes concurrentes, las peticiones esperaban conexión.

## Cambio

| Variable | Valor por defecto | Rango | Efecto |
|---|---|---|---|
| `PLACEMENT_GO_TOKEN_CACHE_TIME` | `300` | `-1` (desactivado) o `1..86400` s | Igual que `token_cache_time` de keystonemiddleware |
| `PLACEMENT_GO_DB_MAX_CONNECTIONS` | `16` | `1..256` | Conexiones abiertas e inactivas del pool de `serve` |

- El caché es el mismo que usa Glance Go: LRU acotado a 10 000 entradas, guarda
  solo el hash SHA-256 del token, nunca cachea fallos y revisa la expiración del
  token en cada uso.
- El modo `migrate` conserva el pool pequeño de `mysql.Open`.
- Las variables de Glance no configuran Placement, y `DB_MAX_CONNECTIONS` no
  aplica a Glance. Lo verifican las pruebas de `internal/config`.
- El log de arranque informa los dos valores efectivos.

## Efecto sobre pruebas anteriores

**Revocación (B12).** Con el caché activo, un token que Go ya validó sigue
aceptado hasta 300 s después de revocarse. Es el mismo comportamiento que
keystonemiddleware en el original. La fase `revoked` de B12 validaba el token en
Go *antes* de revocarlo y esperaba 401. Para repetirla tal cual hay que usar
`PLACEMENT_GO_TOKEN_CACHE_TIME=-1`, o bien comparar ambos servicios con el token
ya visto por los dos.

## Resultado en la VM (2026-11-20)

Protocolo `placement-compact-v2-reads`: 36/36 ventanas válidas, 0 errores,
original restaurado y VM apagada. [Evidencia](../../evidence/sprint5/benchmarks/placement-compact-v2-reads-20261120/README.md).
Medianas de tres pares; entre corchetes, el rango observado del cociente pareado.

| Carga | Conc. | Python ops/s | Go ops/s | Go/Python | p50 Py/Go (ms) | p99 Py/Go (ms) |
|---|---:|---:|---:|---:|---:|---:|
| provider | 1 | 302 | 2064 | 6,83 [6,75–6,84] | 3,1 / 0,4 | 5,0 / 0,7 |
| provider | 16 | 721 | 3516 | 4,88 [4,87–4,88] | 21,4 / 4,2 | 36,6 / 8,6 |
| inventories | 1 | 191 | 1403 | 7,28 [7,24–7,47] | 5,0 / 0,6 | 7,7 / 1,1 |
| inventories | 16 | 440 | 3285 | 7,47 [7,14–9,52] | 35,3 / 4,5 | 52,6 / 9,2 |
| allocations-read | 1 | 201 | 1339 | 6,67 [6,16–7,02] | 4,6 / 0,6 | 8,0 / 1,8 |
| allocations-read | 16 | 519 | 3289 | 6,26 [6,23–6,45] | 29,2 / 4,6 | 48,5 / 9,5 |

CPU por petición correcta: Go/Python 0,060–0,065. PSS: Python ~182–185 MiB,
Go ~15–16 MiB.

En compact-v2 (31-oct, sin caché y con pool de 4) Go hacía 0,61–1,04× el
throughput de Python en estas celdas; ahora hace 4,9–7,5×. Python mide
valores similares a los de octubre (provider c1: 280 → 302 ops/s), lo que
indica un entorno comparable. La desventaja anterior se debía a la
validación de tokens contra Keystone en cada petición y al pool reducido, no
al lenguaje.

Límites: tres pares por celda y dos concurrencias, sin significancia
estadística. Ambos servicios cachean tokens (Python en memcached, Go en
memoria del proceso), pero workers, pools y configuración MySQL siguen siendo
distintos. Las cargas candidates, claim-cycle y nova-mix no se repitieron; sus
resultados de compact-v2 corresponden al binario anterior.

## Verificación

`go test -race ./...` y `make check` aprobados. Pruebas nuevas:
`TestPlacementTokenCacheTime`, `TestPlacementDatabasePoolSize` y
`TestConfigurePoolSetsOpenAndIdleLimits`.
