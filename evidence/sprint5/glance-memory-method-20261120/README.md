# Método de medición de RAM de Glance: validación (2026-11-20)

VM local. En el mismo instante, con 8 clientes por servicio listando imágenes, se compararon las fuentes que usa la campaña con herramientas estándar ([validation.json](validation.json), [script](memcheck.py)):

| Servicio | PSS del colector (`smaps_rollup`) | PSS de `smem` | `anon` del cgroup v2 | Σ `Pss_Anon` | Σ RSS |
|---|---:|---:|---:|---:|---:|
| Glance (3 procesos de uWSGI) | 233,9 MiB | 233,6 MiB | 229,7 MiB | 229,7 MiB | 302,0 MiB |
| Glance Go (1 proceso) | 16,0 MiB | 16,2 MiB | 8,0 MiB | 7,8 MiB | 16,0 MiB |
| memcached (caché de tokens del original) | 10,0 MiB | 10,0 MiB | 9,6 MiB | 9,6 MiB | 13,3 MiB |

Conclusiones:
- **La PSS del colector coincide con `smem`** (diferencia de 0,3 MiB o menos). Es la métrica principal.
- **El `anon` del cgroup coincide con la suma de `Pss_Anon`.** Mide la memoria privada de toda la unidad, sin contar dos veces lo compartido, y por ser una sola lectura se muestrea a 10 Hz (`scripts/benchmarks/glance_memory.py`, verificado: 30 muestras en 3 s).
- **La suma de RSS sobreestima al original en ~68 MiB**, porque cuenta la memoria compartida de sus 3 procesos. Queda solo como cota superior.
- **La caché de tokens del original vive en memcached (~10 MiB)**, que no entra en su PSS. Se registra como grupo de control.
- **En Glance Go**, la PSS de 16 MiB incluye ~8 MiB del código del binario mapeado en memoria; su memoria privada (`anon`) es de 8 MiB.
