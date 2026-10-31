# Campañas posteriores: Glance y sustitución conjunta

Solicitud del usuario, 2026-10-30 (America/Santiago): después de la campaña actual
de Placement, crear una campaña de Glance Go y otra con ambos servicios Go.
Estado: protocolos y secuencia preparados; runners específicos, pilotos y
mediciones oficiales pendientes. No se presenta planificación como ejecución.

## Secuencia y exclusión del laboratorio

1. Terminar la campaña Placement B17/B18 ya activa, sin editar sus fuentes,
   HEAD congelado, protocolo, supervisor, configuración, datos o procesos.
2. Verificar cierre aceptado, restauración, limpieza, checks/CI y liberación de
   la VM. Un fallo requiere reparar la causa antes de usar el laboratorio.
3. Implementar y validar Glance Python frente a Go, con Placement Python fijo.
4. Restaurar y verificar originales. Ejecutar después la campaña conjunta.
5. Restaurar ambos originales, retirar sólo recursos propios, apagar la VM y
   publicar resultados separados con sus límites y trazabilidad B16/B17/B18/B23.

El worktree de preparación es independiente de la ejecución Placement. No se
ejecutan make check, compilaciones ni campañas paralelas durante las ventanas
actuales. El seguimiento se programa en este mismo chat. La cola no ejecuta un
protocolo sin sus instrumentos y pilotos aprobados.

## Puerta de entrada común

La captura/Excel dice B20/B25/B26 Hecho; esta declaración no identifica por sí
sola el código que validó Glance. Localizar las revisiones integradas y sus
evidencias reales antes de elegir una base. Los documentos B25 de este checkout
todavía registran validaciones vivas pendientes. Contrastar esa discrepancia;
no asumir un boot con Glance Go por el estado del backlog. Si falta código o
evidencia, completar y probar las precondiciones en el laboratorio autorizado;
informar cualquier dependencia que no se pueda resolver.

Registrar SHA de ambos servicios Python, commits/binarios/toolchains Go,
manifiestos de dependencias, XML de la VM, hardware, kernels, Apache, procesos,
workers, MySQL, stores y espacio libre. Mantener Keystone real, Nova/red/flavor
sin cambios entre variantes. Código Go usa bases y store propios, nunca el
backend original. Hacer una precomprobación funcional con datos nuevos para
metadatos, upload/download y boot. Confirmar durabilidad de bytes tras reinicio.

Los recursos deben pertenecer al ensayo mediante IDs y ledger explícitos. Se
permite crear y eliminar por API recursos sintéticos propios; nunca SQL de
escritura sobre las bases originales, ni limpieza global de imágenes/cachés.
Tokens, DSN, app credentials, backups y canales SSH privados quedan fuera de Git.
Usar lock exclusivo del laboratorio, rollback de rutas con temporizador y
restauración probada incluso al fallar. Recoger evidencia de ambos servicios
originales activos y del backend propio retirado antes del apagado normal.

Congelar manifiesto, orden aleatorio y protocolo por hash antes de medir. Si el
piloto exige cambiar límites o cargas, crear otra revisión antes de comenzar;
conservar la anterior, sin alterar una campaña en marcha ni ocultar intentos.

## Campaña 1: Glance aislado

Parámetros: [glance-protocol-v1.json](glance-protocol-v1.json). Nueve cargas,
cuatro concurrencias, cinco pares Python/Go: **360 ventanas oficiales**. Cada
ventana tiene 30 s de calentamiento y 120 s de carga: mínimo 15 horas, más
preparación, drenaje, calibración, limpieza y controles. El piloto de 36 ventanas
usa todas las cargas y concurrencias 1/16; queda excluido del resultado oficial.

Mantener Placement Python activo. Una implementación Glance por vez bajo la
misma ruta pública; verificar en los logs qué implementación respondió. Preparar
100 imágenes privadas del ensayo con metadatos equivalentes, en esquemas/stores
separados. Limitar el listado al conjunto propio con filtros admitidos por ambas
implementaciones y verificar paginación/orden completos contra el manifiesto.

| Carga | Operación y validación |
|---|---|
| Catálogo listado | GET de las imágenes propias; IDs, tipos, estados y paginación esperados. |
| Catálogo consulta | GET de imagen; campos estables completos e identidad/owner correctos. |
| Ciclo de metadatos | POST queued, PATCH y GET de comprobación, DELETE y GET 404; UUID propio por ciclo. |
| Upload 1/16/64 MiB | POST propio, PUT streaming, GET metadata active y DELETE; validar longitud y hashes de contenido. |
| Download 1/16/64 MiB | GET streaming de imagen active preparada; comprobar bytes, longitud y SHA256 completos por respuesta. |

Generar payloads deterministas fuera de las ventanas, sin guardar imágenes en
Git. Streaming con memoria acotada también en el cliente. En upload, el estado
active y los hashes se verifican en cada ciclo; descargar y cotejar el payload
de cada worker antes/después de cada ventana. La prueba funcional previa exige
round-trip completo para cada tamaño. No confundir comprobación por metadatos con
haber vuelto a descargar cada upload de la carga.

Cada worker conserva una sola operación/ciclo pendiente. En uploads, creación,
verificación y DELETE forman parte del ciclo y su tiempo: publicar tanto la
latencia del PUT como el goodput de ciclos completos. No llamar throughput puro
de upload a un ciclo que incluye metadatos. Descargar publica bytes correctos/s
y MiB/s además de requests/s; registrar time-to-first-byte y tiempo completo.
DELETE debe comprobar liberación del blob propio fuera de la medición mediante
el adaptador del backend y ledger, sin barrer archivos ajenos.

Registrar p50/p95/p99, errores, latencia por operación, ciclos correctos/s,
MiB/s, CPU, CPU/op y PSS/RSS por cgroup. Para PUT, separar validación/cleanup del
tiempo de transferencia y conservar ambos. Pools, workers y cachés se registran;
el límite de transferencias Go previsto es 16 para no rechazar por admisión la
carga normal de 16 workers. Aplicarlo sólo antes del piloto, declarar la
configuración efectiva y conservar el comportamiento por defecto como dato del
despliegue. Saturación/rechazos 503 son pruebas adicionales, no éxitos normales.

Disco libre mínimo 8 GiB y store propio acotado a 4 GiB. Limpiar cada ciclo,
muestrear espacio fuera del temporizador de requests; cualquier acumulación,
falta de espacio, hash incorrecto, timeout o error invalida la ventana y conserva
la causa. No reducir tamaño, concurrencia o repeticiones para obtener un pase.
Si el disco no alcanza, revisar el diseño antes de congelar otra revisión.

## Campaña 2: Placement y Glance con Nova

Parámetros: [joint-protocol-v1.json](joint-protocol-v1.json). Diseño 2×2 con
Placement Python/Go y Glance Python/Go. Aunque el objetivo principal es ambos
Go, los cuatro estados permiten separar sus efectos en este entorno:

| Variante | Placement | Glance |
|---|---|---|
| Referencia | Python | Python |
| Sustitución Placement | Go | Python |
| Sustitución Glance | Python | Go |
| Sustitución conjunta | Go | Go |

Nova, Keystone, Neutron, red, imagen, flavor y VM permanecen fijos. Para cada
variante: cinco repeticiones, lotes de 1/2/4 boots concurrentes y perfiles de
caché fría/caliente. Total **120 bloques y 280 boots medidos**, más precalentamiento
y piloto de 16 bloques/24 boots. Aquí la unidad de repetición es el lote; no
se usa una ventana HTTP de 120 s para sustituir el tiempo real hasta ACTIVE.

Fijar una imagen CirrOS local con hash, tamaño y formato, sin descargas durante
la medición. Antes del diseño oficial verificar cuatro boots de preflight, uno
por variante, incluyendo consulta de imagen, reserva y liberación correctas.
El bloque usa una barrera común; mide POST de Nova hasta ACTIVE, polling con
cadencia fija registrada y plazo 300 s. Conservar BUILD/ERROR/timeouts también.
El throughput del lote es boots correctos sobre su duración; reportar además
tiempos individuales, con incertidumbre por sondeo y sin tratarlos como lotes
independientes. No exigir ping/SSH de la instancia para sustituir la evidencia
de ACTIVE y uso de servicios, salvo que se defina otra métrica expresamente.

Cada lote usa imágenes y servidores propios nuevos. Caché fría: UUID nuevo y
evidencia de transferencia desde Glance para cada arranque que se etiquete frío.
Si Nova reutiliza caché y no hay transferencia verificable, el bloque no cumple
ese perfil; no se elimina una caché global para forzarlo. Caché caliente:
preboot exitoso fuera de la ventana, borrar ese servidor, confirmar la caché y
usar la misma imagen en el lote medido. Registrar bytes realmente servidos:
un boot caliente puede no transferirlos y no mide throughput de Glance.

Durante cada lote demostrar ambos servicios por proceso/ruta/requests
correlacionadas: llamadas Nova→Placement, imagen obtenida de Glance y allocations
que pertenecen al servidor. Cuando corresponda, originales detenidos en sus
rutas de sustitución y no utilizados como fallback. Una ruta Go sin requests
no demuestra sustitución conjunta. Tras borrar, no quedan instancias,
allocations, puertos ni imágenes/archivos propios huérfanos; registrar estado
de compute y retorno a capacidad inicial. Un ERROR/NoValidHost en el escenario
normal no cuenta como éxito de rendimiento.

Muestrear cada cgroup de Placement, Glance, Nova, DB, Apache y Keystone por
separado y recoger total VM como medida distinta; no sumar RSS con páginas
compartidas ni atribuir todo el costo a Go. Medir la ventana real de arranque
y cobertura/intersección CPU con el reloj común. Se publican p50/p95 y, sólo
si hay soporte muestral suficiente, p99; cinco lotes por celda no justifican un
p99 de boot fiable. Exponer la distribución y los lotes en vez de inventarlo.

## Estadística, instrumentos y cierre

Desarrollar runners Glance y conjunto con contratos explícitos, guardas de
propiedad, límites, ledgers, recuperación y salida no cero ante resultados
inválidos. Reutilizar autenticación/recolectores revisados cuando encajen, sin
reutilizar ciegamente el validador ni el análisis específico de Placement.
Pruebas relevantes: respuesta 2xx con bytes erróneos no pasa; upload parcial,
timeouts y fallo de cleanup conservan evidencia; la matriz no omite variantes;
un boot sin traza Go o con allocations huérfanas no pasa. Después de terminar
Placement, correr make check y pruebas específicas antes de pilotos reales.

Bootstrap pareado por repetición/bloque, 10000 remuestras, intervalo 95%, con
mediana, dispersión, diferencias y ratios por celda. La comparación conjunta
reporta las cuatro variantes y, si el diseño/medida lo permite, interacción
entre sustituciones con su incertidumbre. No sumar mejoras individuales como
predicción del conjunto, ni atribuir diferencias únicamente al lenguaje.
Publicar errores y todas las repeticiones, incluyendo fallos e intentos retenidos.
No aceptar una campaña por obtener una mejora: equivalencia y calidad de los
datos determinan el pase. Un piloto nunca acredita las ventanas oficiales.

Evidencia en directorios nuevos Sprint 5, separados por protocolo/run:
manifiesto, hashes y orden; requests/tiempos saneados; muestras HTTP/bytes y
CPU/PSS; estados Nova y allocations; snapshots inicial/final; limpieza,
restauración y apagado; análisis/tablas/figuras y límites. Git recibe metadatos,
muestras comprimidas revisadas y hashes, nunca imágenes, secretos o backups.
Commits por tarea/campaña y PR revisable, sin merge automático. B17/B18 actuales
conservan aceptación limitada a Placement; la evaluación global exige estas dos
campañas adicionales. B23/B24 integran sus resultados sólo después de validar.
