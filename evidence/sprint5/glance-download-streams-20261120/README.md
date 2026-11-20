# Límite de descargas simultáneas de Glance Go (2026-11-20)

Medición en la VM local (4 vCPU), con Glance Go en el catálogo oficial y el binario `54fcb48f…`:
- 16 clientes descargan sin pausa a través de `/image` (Apache), y cada descarga se valida con MD5 y SHA-512;
- 5 s de calentamiento y 2 ventanas de 20 s por cada valor de `GLANCE_GO_MAX_DOWNLOAD_STREAMS` ([datos](streams.jsonl)).

| Límite | 16 MiB | 64 MiB |
|---|---:|---:|
| 2 (anterior, `max(2, GOMAXPROCS/2)`) | 879 / 909 MiB/s | 749 / 742 MiB/s |
| 4 (`GOMAXPROCS`) | 909 / 906 MiB/s | **990 / 991 MiB/s** |
| Sin límite (como el original) | 901 / 903 MiB/s | 969 / 967 MiB/s |

No hubo errores en ninguna ventana. El valor por defecto pasa a `GOMAXPROCS`: con 64 MiB rinde un 33 % más que antes y un 2 % más que sin límite, y con 16 MiB no cambia. Después del cambio, el E2E compartido con Nova pasó 5/5.
