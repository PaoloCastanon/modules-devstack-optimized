# Glance Go como servicio de DevStack (2026-11-22)

Revisión `1cb474f9`, rama `feat/glance-official-only`. Glance Go se instala con
el plugin `glance-go` ([uso](../../../docs/glance/devstack-plugin.md)) como
`devstack@g-go`, en lugar de g-api. Ya no hay instancias temporales ni
instaladores propios.

## Resultados

| Verificación | Resultado |
| --- | --- |
| Plugin, fase `stack install` sobre el DevStack de la VM | Go 1.27.4 verificado por sha256; binario compilado en 22 s |
| Plugin, fase `stack extra` | `devstack@g-go` activo y habilitado; `devstack@g-api` detenido y deshabilitado; `/image` → Glance Go; 23 imágenes del original adoptadas; arranque en 165 ms |
| Suite `glance_devstack_suite.py` (B26 + E2E del catálogo compartido, [suite/](suite/)) | 13/13 corridas, 11/11 casos cada una (una con 1, 16 y 64 MiB) |
| Integración MySQL con `-race` contra el DDL real ([integration/](integration/)) | 9/9 |
| `make check` | OK |
| Carga dirigida: 16 clientes con POST + PUT 64 MiB + DELETE durante 180 s | 580 ciclos, 0 errores, 0 deadlocks; DELETE p50 143 ms, p99 513 ms |
| Arranque | 35 ms (antes, 13 s con 38 530 filas de contabilidad) |

## Límite

Las fases del plugin se probaron sobre el DevStack ya instalado, con las mismas
funciones que usa `stack.sh`. Falta un `stack.sh` limpio con
`enable_plugin glance-go`.
