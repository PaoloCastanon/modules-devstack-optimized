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

**Rendimiento.** No hay mediciones nuevas. Para acreditar el efecto hay que
repetir en la VM las celdas provider, inventories y allocations-read del
protocolo compact-v2, declarando la nueva configuración.

## Verificación

`go test -race ./...` y `make check` aprobados. Pruebas nuevas:
`TestPlacementTokenCacheTime`, `TestPlacementDatabasePoolSize` y
`TestConfigurePoolSetsOpenAndIdleLimits`.
