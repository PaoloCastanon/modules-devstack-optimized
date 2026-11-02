# B16 — protocolo experimental Placement, revisión 1

Actualización 2026-10-31: el usuario solicitó campañas de menor duración. La
[revisión compacta v2](compact-campaigns-v2.md) y su JSON definen el siguiente
ensayo preliminar: 72 ventanas, tres pares, concurrencias 1/16 y 10/30 s.
Esta página conserva la revisión 1 histórica; no se mezclan sus intentos ni se
presenta la cobertura de v2 como las 240 ventanas de v1.


2026-10-29 (America/Santiago). Revisión 1 para la primera campaña Placement
Python/Go. Cierre del protocolo condicionado al piloto y sus pruebas registradas
en evidence/sprint4/benchmarks/b16-20261029/. B17/B18 no se acreditan por calibrar
el instrumento. No se exige una mejora para aceptar un resultado experimental.

## Preguntas y unidades

Comparar latencia HTTP, operaciones correctas/s, CPU y memoria de Placement bajo
las mismas operaciones y estado del laboratorio. Una solicitud es la unidad de
latencia; una repetición independiente es la unidad de variación entre corridas.
El tiempo de boot Nova es una medición end-to-end aparte. La campaña mantiene
Glance original; Glance Go y B26 conservan sus propias pruebas.

## Puerta de entrada

Antes de cualquier medición oficial, B13/B14 deben demostrar sustitución y boot,
y B15 dos ejecuciones de integración con restauración. Deben constar los commits
exactos, código/hash de instrumentos, versión de protocolo, configuración efectiva,
semilla, baseline inicial y final y resultados de cleanup. Una diferencia funcional
impide presentar la corrida como comparación de rendimiento válido. No sustituir
Keystone por dobles ni deshabilitar auth para conseguir throughput.

## Ambiente controlado

- Implementaciones dentro de tt-openstack-lab, misma asignación de cuatro vCPU y
  aproximadamente 8 GiB RAM; verificar valores al ejecutar. Misma ruta Apache
  pública /placement y mismo generador externo al presupuesto del servicio.
- Una implementación bajo carga por vez. Mantener versiones Nova, Glance,
  Keystone, red y actividad de control iguales. Registrar tanto SHA Python y
  binario/toolchain Go como kernel, hipervisor, discos, afinidad y límites.
- MySQL propio Go y base original Python separados. Registrar versión, InnoDB,
  durabilidad, binlog, pool/conexiones, índices y hardware. Si no son equivalentes,
  tratarlo como factor del despliegue, no atribuir la diferencia sólo al lenguaje.
  No ejecutar SQL de escritura en la DB original ni alterar sus grants.
- Registrar procesos/threads Python, GOMAXPROCS/pools Go, logging, timeouts,
  configuración de caché de token, estado de Memcached y cargas ajenas. No asumir
  igualdad de costos auth: Go revalida usuarios y el middleware Python cachea.
- Registrar relojes host/VM y su diferencia/incertidumbre antes/después. Latencia
  usa reloj monotónico del cliente. UTC sólo alinea ventanas de procesos entre
  máquinas. Mantener pausas y no ejecutar make check, compilación o migraciones
  durante las ventanas oficiales. Preparación y reset quedan fuera de ellas.

## Semilla y cargas

La semilla referencia un manifiesto con IDs explícitos. Usar providers sintéticos
propios y una clase CUSTOM única para excluir el compute real de las consultas de
candidates. Los providers son planos, tienen VCPU/MEMORY_MB/DISK_GB, OWNER_NOVA,
la misma capacidad/constraints por clase y la misma ocupación en ambos servicios.
Registrar cantidades y consultas completas, sin token. Crear por API y borrar por
propiedad verificada. Para owner de allocations en Python reutilizar un project/
user ya presente en Placement y evitar filas globales imposibles de limpiar por
HTTP. No reutilizar la DB original como backend de Go.

Primera matriz: un conjunto de 16 providers sintéticos, todos idénticos, con VCPU
4, MEMORY_MB 1024, DISK_GB 10, allocation_ratio 1, reserved 0, min/step 1 y max=total;
clase CUSTOM de selección con total 64 y ratio 1. Sin allocations iniciales.
Repeticiones y concurrencias comparten la misma capacidad y estado inicial.
Comprobar hashes de snapshots normalizados sólo para IDs/origen/fechas permitidos.
No normalizar generaciones, errores o cantidades. Medir escalas de catálogo
adicionales exige una revisión de protocolo antes de incorporar sus resultados.

| Escenario | Operaciones/versiones y validación |
|---|---|
| Provider | GET provider 1.14; UUID, generación y JSON completo contra snapshot esperado |
| Inventories | GET inventories 1.26; generación, clases y constraints exactos |
| Allocations read | GET consumer 1.28; consumer propio con estado sembrado y estable |
| Candidates | GET candidates 1.36 con resources VCPU:1,MEMORY_MB:512,DISK_GB:1,CUSTOM:1 y root_required=OWNER_NOVA,!COMPUTE_STATUS_DISABLED; validar requests/summaries contra el estado sembrado |
| Claim cycle | Consumer propio por worker: PUT allocations 1.36 con consumer_generation null, GET 1.28, DELETE 1.0; reservar una unidad VCPU y CUSTOM. Cada vuelta comienza/termina vacía y verifica datos/generación y 204/200/204 |
| Mezcla Nova | Ciclo fijo de 16 requests: 6 candidates, 4 provider, 2 inventories, GET consumer vacío, PUT claim, GET consumer lleno, DELETE. Registrar también latencia/status por operación |

La capacidad evita agotar recursos en el escenario normal de 16 workers, con
provider asignado por índice. Conflictos CAS y falta de capacidad son escenarios
funcionales separados; nunca contar un 409/400 como operación normal exitosa.

El instrumento `scripts/benchmarks/placement_http.py` implementa las lecturas
individuales con snapshot esperado completo, lectura acotada y TLS verificado.
Cada worker tiene su conexión keepalive. Warm-up precalienta servicio/cachés;
las conexiones se crean de nuevo para la ventana medida, incluyendo su primera
apertura. El soporte de claim/mezcla y el orquestador de campañas se implementan
con B17, se validan y se pilotan antes de su primera medición oficial. Añadir esos
escenarios no autoriza cambiar esta matriz sin revisar/congelar el protocolo.

## Diseño de corrida congelado

Parámetros en protocol-v1.json: concurrencias 1/4/8/16, warm-up 30 s, carga 120 s,
timeout por solicitud 10 s y cinco repeticiones por escenario/implementación/
concurrencia. Pilotos cortos se etiquetan pilot y no se agregan a mediciones.

Carga closed-loop: una solicitud pendiente por worker, sin think time artificial.
Medir throughput alcanzado y latencia bajo esa carga; no inferir capacidad para
una tasa externa fija ni ocultar coordinated omission. Para estudiar cargas de
llegada abiertas habría que añadir otro diseño e instrumento.

Alternar el orden Python/Go por bloque usando semilla 20261029 y guardar la
secuencia antes de empezar. Mismo recurso/token durante cada par; refrescar tokens
fuera de las ventanas, con la misma anticipación a expiración. Separar escenarios
de caché fría y régimen estable; esta primera matriz es régimen estable con
warm-up. No borrar cachés del sistema o de otros servicios arbitrariamente.

La ventana empieza al liberar una barrera con todos los workers preparados. Se
inician requests hasta el deadline monotónico; se drenan las ya iniciadas. El
denominador es max(duración planificada, última respuesta completa menos inicio),
excluyendo teardown de threads. Cada request guarda offset/latencia ns, status,
validación, bytes y tipo de error, sin headers/response body. Guardar intervalo
UTC de cada fase para alinear el recolector. Un timeout sigue siendo una muestra.

## Métricas y análisis

- Latencia: p50/p95/p99 por nearest rank, índice ceil(p*n), de requests válidas;
  publicar también distribución de todas las solicitudes y tasa de error.
  Menos de 1000 respuestas válidas marca p99 con soporte de cola insuficiente
  (menos de diez observaciones esperadas en el 1% superior).
- Throughput: completadas/segundo y correctas/segundo por separado. JSON incorrecto,
  status inesperado, timeout o error de transporte no cuenta como operación correcta.
  Verificar snapshots antes/después y cleanup incluso cuando HTTP fue exitoso.
- CPU: diferencia de ticks utime+stime de todos los PIDs del servicio dividida por
  CLK_TCK. Reportar CPU-segundos, fracción de núcleos asignados y CPU/op correcta.
  No agregar ciegamente todo Apache, la VM, DB, Keystone o el generador.
- RAM: PSS agregado de los procesos, reposo, serie temporal y pico observado;
  complementar con RSS declarando páginas compartidas. No llamar pico absoluto
  al máximo muestreado. Mantener separados Go heap/GC y consumo total del servicio.
- Muestreo: 1 s y PID+start_ticks para detectar reciclaje/reutilización. Identificar
  todos los workers mediante cgroup antes/después; cambio de composición invalida
  la comparación estática y requiere repetir con enumeración adecuada. Si no hay
  permiso PSS, registrar ausencia y no mezclar RSS/PSS en una misma comparación.
- Alineación: recolectar durante warm-up/carga/drain, seleccionar muestras interiores
  a la ventana de carga según UTC y relojes verificados. Para CPU/op usar las
  operaciones completadas dentro de ese mismo intervalo común. Reportar duración
  cubierta y resolución/incertidumbre de alineación; no atribuir todo el piloto a
  una sola corrida o incluir warm-up en CPU de la carga.

Conservar muestras y resúmenes por repetición. Mostrar mediana de cada métrica,
dispersión e intervalo de confianza bootstrap del 95% por bloques/repeticiones
(10000 remuestras, semilla registrada); un n=5 tiene incertidumbre limitada y se
declara. Comparar diferencias/ratios de pares; no bootstrapear requests como si
fueran repeticiones independientes. No promediar p95/p99 para llamarlos percentiles
globales. Se puede publicar un percentil combinado calculado de todas las muestras,
etiquetado como tal y separado del resumen por corrida.

## Validez, evidencia y repetición

Invalidar una corrida oficial si cambia la semilla/configuración, falla una
respuesta normal, falta un worker/muestra esencial, expira el token, hay carga
ajena no controlada o no se verifica el estado final. Conservarla con motivo y
repetir el bloque después de corregir la causa; no excluir una corrida por ser
lenta. Los errores de escenarios negativos se evalúan por el contrato previsto.
No cerrar B17/B18 con fixtures, un piloto, valores fabricados o una sola corrida.

Por corrida: manifest con hashes/versiones/estado, samples.jsonl, recursos JSONL,
report.json, snapshot público de referencia, evidencia de control y limpieza.
Credenciales, DSN y backups de configuración van en un directorio privado fuera
del repo y se eliminan al terminar. Guardar datos posteriores en Sprint 5; el
piloto B16 permanece en Sprint 4. Restaurar el original y apagar VM entre sesiones.

El piloto B16 comprueba el generador y el muestreo real con el original; no prueba
por sí solo la mezcla, escrituras, equivalencia de despliegue ni el boot con Go.
Su evidencia y los tests de fixture muestran por separado lo comprobado. Cualquier
cambio futuro de límites, estadística, semilla o métrica requiere nueva revisión
antes de medir; conservar la versión anterior y sus hashes.
