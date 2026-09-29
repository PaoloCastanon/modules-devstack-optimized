# Estrategia de pruebas

| Nivel | Verificación | Momento / evidencia |
|---|---|---|
| Unit Go | generación, disponibilidad, transiciones de imagen, hash, errores | Sprint 3+, `go test` |
| API | método/ruta, microversiones, JSON, headers, auth positiva/negativa, errores | fixtures contra original y Go |
| Compatibilidad | misma request en ambos, diff normalizado de status, body y efectos | `tests/compatibility`, reporte por caso |
| Integración | Nova Scheduler/Compute ↔ Placement; Nova Compute ↔ Glance | logs sanitizados, catálogo, VM |
| End-to-end | Glance Go + Placement Go + Nova original → crear VM | boot ACTIVE, imagen consumida, allocations correctas |
| Benchmark | latencia p50/p95/p99, throughput, CPU, RSS RAM, tasa de error | etapa posterior; repeticiones, misma carga/host |

En Sprint 2 el gate observable es `make sprint2-verify` para sintaxis y consistencia, más `make smoke`/`make baseline` en VM. Un gate de sintaxis no valida RF01 ni RF10. En benchmarks posteriores se fijarán warm-up, tamaño de imagen, concurrencia y número de repeticiones antes de medir; no hay resultados todavía.
