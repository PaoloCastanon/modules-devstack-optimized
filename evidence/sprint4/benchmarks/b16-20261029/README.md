# B16 — protocolo y piloto del instrumento

Fecha local 2026-10-29. Base integrada B12 c2cadc7 más plan 144de66.
B16 completa la metodología del alcance Placement; no son resultados B17/B18.

- Protocolo: docs/benchmarks/methodology.md y protocol-v1.json. Concurrencias
  1/4/8/16, warm-up 30s, carga 120s, cinco repeticiones y muestreo 1s para la
  futura campaña oficial, después de B13/B14/B15. Definidos semilla, cargas,
  ventanas, percentiles, errores/goodput, CPU/PSS e incertidumbre.
- Instrumento HTTP real de lectura: conexión por worker, reloj monotónico,
  barrera, validación de JSON completo contra snapshot esperado y salida no cero
  si hay errores. Guarda muestras sin token ni response body. Las extensiones
  claim/mezcla y su propio piloto se completan con B17 antes de medirlas.
- Recolector Linux real: todos los PIDs enumerados del servicio, ticks CPU,
  PSS y RSS, detección de PID ausente/reutilizado. No captura argv/environment.
- Tests rojos al faltar el módulo; seis tests finales PASS. Incluyen sockets
  reales, respuesta 200 con estado erróneo, rechazo de credenciales en evidencia,
  errores rápidos excluidos de goodput, percentiles, output existente, lectura
  real de /proc y ventana NaN rechazada. make check PASS (incluye ambos dominios).

pilot-01 y pilot-02 usan el Placement Python desplegado, Keystone real y el mismo
provider compute, sólo GET provider 1.14. No modifican recursos originales.
Cada piloto pasa las cuatro concurrencias con warm-up 1s/carga 4s y cero errores.
El primero conserva el instrumento previo a explicitar ventana UTC y a excluir
teardown/agregación del denominador. Su fuente exacta está archivada como .txt.
El segundo identifica el instrumento final por SHA; añade start_utc/end_utc por
fase y mide hasta el deadline/respuesta final. Las ocho corridas no se mezclan
ni se presentan como comparación Python/Go. Los datos crudos y hashes se conservan.

El recolector cubre 25s alrededor de todo el piloto; no atribuir su CPU completa a
una concurrencia concreta. PIDs 931/1053/1054 corresponden al master y dos workers
uWSGI de la unidad Placement, con PSS disponible. Este piloto no iguala todavía
los despliegues ni valida escrituras o mezcla. B13 prepara Go dentro de la VM y
B17 agrega campañas controladas. No hay afirmación de mejora de rendimiento.

Las credenciales temporales del piloto quedan privadas y en uso por B13; su
cleanup y apagado de VM se registran al cierre de esa sesión. El directorio
privado del generador fue eliminado en finally. No se cambió Excel ni S2.

Cierre posterior de la sesión: evidence/sprint4/placement/b14-20261029/ acredita
retirada del runtime/identidad/perfil propios, original restaurado y VM apagada.
