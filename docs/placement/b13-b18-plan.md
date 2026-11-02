# Plan B13–B18: integración Nova y evaluación de Placement

Propuesta del 2026-10-29, basada en el cierre B12 `5c682af`. Esta entrega prepara
el trabajo; no ejecuta sustituciones, pruebas de boot ni benchmarks. La VM
permanece apagada. No cambia estados, puntos, responsables ni fechas del Excel.

## Alcance y dependencias

Fuente de backlog: [planificación aceptada de Sprint 2](../../Planificacion_Trabajo_Titulo_OpenStack_Placement_Glance_Sprint2.xlsx),
hoja Backlog, filas 16–21, columnas A–M; requisitos RF09/RF12/RNF02/RNF03 en
Requerimientos, filas 12, 15, 17 y 18. B13–B16 conservan S4 y B17–B18 conservan S5.
B13–B15 y B17–B18 corresponden a ambos integrantes; B16 a Paolo. Las estimaciones
originales son 8/8/8/5/5/5 puntos; aquí no se convierten en días ni nuevas fechas.

El punto de partida es [B07–B12 aceptado en su subconjunto](b12-acceptance.md):
providers planos, inventarios, catálogos/traits, allocations con generaciones,
candidates, MySQL propio y Keystone. B12 pasó 143/143 casos y las regresiones
B07–B10 267/267; B11 verificó durabilidad/restore 9/9. Esto no acredita todavía
que Nova pueda completar el flujo con Go.

La campaña B13–B18 compara **Placement Python y Placement Go**, manteniendo Nova,
Glance original, Keystone y el resto del laboratorio como controles. El boot
usando una imagen servida por Glance Go pertenece a B26, con B20/B25/B14 como
prerrequisitos. La revisión de B25 pasó MySQL real con auth doble; Keystone real,
la evidencia end-to-end y algunos documentos aún requieren cierre propio. Estos
pendientes no se convierten en dependencias nuevas del ensayo Placement.

| ID | Trabajo | Dependencia formal | Cierre verificable |
|---|---|---|---|
| B13 | Sustituir Placement original | B07–B12 | Nova publica y consulta recursos en Go mediante una sustitución reversible, sin cambios sustanciales a sus consumidores |
| B14 | Crear instancia con Placement Go | B13 | VM ACTIVE con traza de candidates, claim, allocations correctas y liberación al borrar |
| B15 | Suite automática de integración | B13 | Casos críticos repetibles, aserciones de estado, informe, limpieza y restauración comprobadas |
| B16 | Metodología experimental | B03 | Protocolo congelado de ambiente, cargas, métricas, repeticiones y análisis antes de las mediciones oficiales |
| B17 | Latencia y throughput | B14, B16 | Datos crudos y resultados comparables Python/Go, con errores y variabilidad visibles |
| B18 | CPU y memoria | B14, B16 | Consumo atribuible a cada servicio bajo las mismas cargas y ventanas de B17 |

Orden propuesto: **B16 → B13 → B14 → cierre B15 → B17/B18**. Se construye el runner
B15 durante B13 y B14, y se termina tras automatizar el boot y los negativos.
B15 aprobado será además una condición operativa para medir, sin alterar las
dependencias formales del libro. B17/B18 se instrumentan en las mismas campañas;
ninguno se declara completo por resultados del otro.

## B13: sustitución reversible

1. Fijar una revisión integrada que contenga B07–B12, sus migraciones/checksums y
   binario reproducible. Verificar los PR pendientes si los hay. Capturar versiones
   y configuración efectiva al iniciar la VM; no inferirlas de los archivos S2.
2. Inventariar consumers, instances, providers, traits, aggregates y topología del
   compute. Contrastar las llamadas reales de resource tracker/scheduler contra
   los perfiles B10/B12: microversiones, filtros y tipos de token. Si aparecen
   árboles, sharing, rutas o autenticación no soportados, registrar la brecha,
   reproducirla y cerrar el contrato antes de sustituir; no simular una respuesta.
3. Preparar binario, supervisor, logs saneados, listener y MySQL propios dentro del
   laboratorio, con cuentas migrador/runtime separadas. Desplegar Go en la misma
   VM que Python permite preparar una comparación sin confundir lenguaje con
   ejecución en hosts distintos. Fijar recursos y puertos tras comprobar disponibilidad.
4. Guardar backup privado y hashes de la configuración que se vaya a cambiar.
   Implementar el cambio y su restauración sobre el punto de entrada de Placement,
   procurando conservar URL pública, identidad y contrato de Nova. Identificar
   primero cómo se sirve Placement: no detener un servidor web compartido que
   también atienda Keystone u otros servicios.
5. Rehidratar el compute en la DB Go a través de las APIs y del resource tracker,
   conservando el UUID del provider usado por Nova. Invalidar cachés mediante
   reinicio controlado de los servicios necesarios. Exigir inventarios/traits/
   aggregates y capacidad correctos antes de admitir nuevas instancias. Un esquema
   vacío no es equivalente si existen VMs o reservas ajenas: aislar un compute sin
   cargas o preparar una migración explícita, sin borrar ni adoptar recursos ajenos.
6. Activar Go y verificar que las llamadas de Nova llegan a su proceso, sin fallback
   a Python. Retirar la aplicación original del punto de entrada durante la prueba;
   conservar la posibilidad de restaurarla. Correlacionar logs por request ID,
   provider y consumer, sin tokens ni contraseñas.
7. Ensayar rollback: limpiar sólo recursos propios del ensayo, restaurar configuración
   y aplicación original, comprobar Keystone/Glance/Nova y validar consultas del
   provider original. Conservar un diff saneado y el resultado de cada comprobación.

Entregables propuestos: `scripts/integration/placement-switch.py`, manifest privado
de restauración, configuración/supervisor sin secretos y runbook bajo docs/placement.
El manifest privado no se versiona. Cierre B13 exige publicación real del compute,
lectura de inventarios/traits, consulta de candidates desde Nova y rollback probado;
no basta con curl directo a Go. El boot constituye la aceptación de B14.

## B14: boot y liberación de recursos

1. Comprobar imagen ACTIVE en Glance original, flavor y red seleccionados por UUID
   explícito. No tomar el primer resultado de un listado. Crear recursos del ensayo
   con prefijo/IDs registrados y plazo de espera acotado.
2. Ejecutar server create con Placement Go activo y aplicación Python fuera del
   punto de entrada. Capturar la transición BUILD→ACTIVE y la selección del compute.
3. Correlacionar scheduler y logs Go: GET candidates, GET/PUT allocations del consumer
   real de la instancia. Verificar VCPU/MEMORY_MB/DISK_GB según flavor y política
   efectiva de disco, provider, project/user, generations y usages persistidos.
4. Borrar la instancia y esperar su ausencia. Comprobar allocations vacías y usos
   devueltos al estado inicial. Registrar reintentos/consumers auxiliares si Nova
   los utiliza; no declarar éxito con reservas huérfanas.
5. Restaurar Placement original y verificar que el laboratorio vuelve a su estado
   funcional. Un fallo conserva traza, estado y resultado de limpieza/restauración.

Entregables: flujo reutilizable de boot/cleanup y evidencia nueva bajo
`evidence/sprint4/placement/b14-<fecha>-<id>/`. Cierre exige ACTIVE, prueba de que Go
atendió la reserva y liberación comprobada; un ACTIVE tomado de S2 no cuenta.

## B15: suite repetible

Crear un comando opt-in con preflight, preparación, switch, casos, limpieza,
rollback y verificación final. Recibe IDs explícitos, credenciales privadas y un
output nuevo; verifica propiedad antes de borrar. Se puede reejecutar en un
laboratorio restaurado. CI ordinario ejecuta guards y pruebas locales; la suite
real usa un target independiente, sin añadir arranque de VM a make check.

Matriz mínima:

| Caso | Aserción |
|---|---|
| Resource tracker | Provider/UUID, inventarios, traits y aggregate membership correctos en Go |
| Boot normal y delete | ACTIVE, allocations del consumer correcto y capacidad liberada |
| Capacidad insuficiente | Nova termina con fallo esperado y no deja allocations huérfanas; original y Go conservan su capacidad inicial |
| CAS y claims concurrentes | Conflicto/reintento válido, sin sobreasignación; resultados y estado coinciden con el contrato |
| Autenticación | Nova/service aceptados; inválido/reader rechazados sin efectos en recursos del ensayo |
| Reinicio Go | Estado persistente y siguiente operación de Nova funcional |
| Fallo durante ensayo | Limpieza acotada y rollback; cualquier fallo de restauración hace fallar la ejecución |
| Restauración | Original operativo y ausencia de recursos propios en ambos lados |

Provocar capacidad insuficiente con un flavor imposible de satisfacer o un conjunto
propio de recursos; no reducir el inventario del compute con cargas ajenas. Para
carreras, combinar claims HTTP deterministas y boots concurrentes hasta el límite
real del laboratorio. Una carrera por HTTP y un boot ejercitan niveles distintos.

Entregables: `scripts/integration/placement_suite.py`, target propuesto
`make placement-integration`, fixtures/manifests sin secretos, JSON por caso y
resumen PASS/FAIL. Reutilizar comparadores B07–B12 y adaptar el boot existente sin
sobrescribir evidence/sprint2. Exigir dos ejecuciones completas consecutivas con
IDs nuevos, limpieza y rollback aprobados. Una omisión obligatoria no cuenta como PASS.

## B16: protocolo previo a medir

Documentar el protocolo bajo `docs/benchmarks/methodology.md` y versionar su
revisión antes de las campañas oficiales. El piloto calibra duración y capacidad;
sus observaciones se etiquetan como piloto y no se mezclan con resultados finales.

- **Ambiente:** misma VM, CPU/RAM asignadas, versiones de Nova/Keystone/MySQL,
  binarios/commits, logging, front-end, presupuestos de CPU/RAM y almacenamiento
  comparables. Registrar workers Python, GOMAXPROCS y pools Go, límites SQL,
  red, índices y configuración de persistencia. Documentar cualquier diferencia.
- **Aislamiento:** una implementación bajo carga por vez, DB/estado propios
  equivalentes y generador fuera del presupuesto del servicio medido. Mantener
  Glance/Keystone/Nova iguales y controlar su actividad de fondo. No comparar las
  mediciones antiguas de Go en el host con Python dentro de la VM.
- **Cargas:** GET provider/inventories/allocations, candidates con el query Nova
  real y PUT/DELETE de allocations. Separar lectura, escritura y mezcla del flujo
  Nova. Generar consumidores propios por cliente; conflictos intencionales forman
  un escenario aparte. La semilla fija tamaño de catálogo, providers, inventarios,
  traits/aggregates y porcentaje de capacidad utilizada para ambos lados.
- **Credenciales/cachés:** Keystone real, identidad y política equivalentes;
  secretos fuera de datos crudos. Separar arranque/caché fría de régimen estable
  y registrar hit rate/configuración del middleware Python. Un 401/403/503 rápido
  no se considera mejor rendimiento ni se sustituye auth por un mock.
- **Diseño inicial propuesto:** concurrencias 1/4/8/16, warm-up de 30 s, ventanas
  de 120 s y al menos cinco repeticiones independientes por combinación. El piloto
  puede ajustar estos valores a la capacidad del lab; congelarlos antes de medir.
  Alternar orden Python/Go mediante secuencia y semilla registradas. Reinicializar
  semilla/estado fuera de la ventana y comprobarlo antes y después de cada corrida.
- **Análisis:** conservar cada observación y cada repetición. Calcular percentiles
  sobre muestras de solicitudes, tasas sobre ventanas efectivas y variabilidad
  entre repeticiones. Dar intervalos de confianza y tamaño muestral; marcar p99
  como insuficientemente sustentado cuando falten muestras. No promediar p95/p99
  para presentarlos como percentiles globales. Declarar timeouts y corridas inválidas
  con motivo; conservarlas en evidencia y no descartarlas por ser lentas.

B16 cierra con protocolo revisable, semilla/cargas definidas, fórmulas, esquema de
resultados y piloto acotado que confirma que el instrumento observa lo esperado.
El criterio formal exige protocolo; el piloto añade la validación práctica del
instrumento. No se requiere que Go resulte más rápido para aceptar el método.

## B17: latencia y throughput

Ejecutar las campañas congeladas después de B14 y B15 aprobados. Guardar latencia
por solicitud con reloj monotónico, endpoint/operación, status, bytes, timeout,
concurrencia, ventana, implementación y run ID. Reportar p50/p95/p99, solicitudes
completadas por segundo, **operaciones correctas por segundo**, tasa de error y
conflictos esperados por separado. Validar las respuestas y el estado posterior;
una respuesta rápida incorrecta invalida la comparación funcional.

El tiempo de boot de Nova se presenta como métrica end-to-end separada de la
latencia HTTP de Placement: incluye red, compute, disco y demás servicios. Guardar
resultados crudos JSONL/CSV y análisis reproducible, con repeticiones, intervalos,
limitaciones de topología/cachés y comparaciones por carga. No inventar un umbral
de mejora para aprobar B17 ni atribuir diferencias al lenguaje sin considerar el despliegue.

## B18: CPU y memoria

Instrumentar las mismas ventanas e IDs de B17, con muestreo inicial de 1 s ajustable
sólo durante el piloto. Incluir todos los workers/procesos que atienden Placement
Python y el proceso Go; registrar PID, inicio, descendientes y límites efectivos.
No atribuir todo Apache o toda la VM a Placement si comparten otros servicios.

Medir CPU-segundos, utilización normalizada por núcleos asignados y CPU por
operación correcta. Medir PSS agregado si está disponible, su evolución/pico y
memoria en reposo; si se usa RSS, declarar el doble conteo potencial de páginas
compartidas y no mezclarlo con PSS. Registrar GC/heap Go y workers Python como
explicación auxiliar. Separar MySQL, Keystone, front-end y generador del costo
atribuido a Placement y conservar también su actividad de control. El muestreo
no se incluye silenciosamente dentro de un único servicio.

Entregar datos de CPU/RAM correlacionados con carga, latencia y throughput,
comparación por repetición e intervalos, y descripción de overhead del recolector.
B18 cierra por comparabilidad y trazabilidad del consumo, no por una reducción obligatoria.

## Entregas y seguimiento

Una unidad/commit por backlog y correcciones acotadas con pruebas rojas/verdes
cuando corresponda. B21 registra decisiones y B22 incorpora los comandos de
instalación/despliegue/prueba; actualizar documentación durante cada bloque sin
atribuirles cierre automático. Guardar evidencia nueva por fecha/run ID en Sprint 4
para B13–B16 y Sprint 5 para B17–B18, con manifest de versiones, requests saneadas,
resultados crudos, informe, cleanup y restauración. Nunca tokens, backups privados,
DSN o contraseñas en Git. Apagar tt-openstack-lab después de cada sesión necesaria.

El primer trabajo al ejecutar este plan será concretar B16 y preparar el preflight/
rollback B13. Esta solicitud autoriza la planificación; la ejecución del plan no
se da por realizada con este documento.
