# Notificaciones de Glance Go: comparación con el oficial y auditoría justa

Fecha 2026-11-19. Rama `feat/glance-notifications`, revisión `011f6d1`, binario
sha256 `fa62bdd7…`. VM local de 6 GiB con Keystone, Nova y RabbitMQ reales.

## Captura del oficial (`captura-oficial-ciclo.json`)

9 mensajes que envió el Glance oficial para POST, PATCH, PUT 1 MiB, GET y
DELETE, leídos de RabbitMQ con una cola privada (IDs reemplazados). Es la
referencia de las pruebas Go.

## Comparación en vivo (`n-glance`, `n-joint`)

La suite B26 corre el mismo ciclo por `/image`, primero con el oficial
(`notifications-original.json`) y después con Glance Go
(`notifications-go.json`). En la comparación (`notifications-compare.json`) hubo
**0 diferencias**: los mismos 9 eventos en el mismo orden, la misma prioridad,
la misma clave `notifications.info`, el mismo emisor y las mismas propiedades
AMQP, con contenidos de igual forma y valores. Solo cambian los IDs y los
tiempos.

| Corrida | Modo | Resultado |
|---|---|---|
| `n-glance` | Glance Go | 12/12 (incluye los dos casos de notificaciones y el boot de Nova) |
| `n-joint` | Glance Go + Placement Go | 16/16 |

## Auditoría justa (`n-audit-1`, `n-audit-2`)

En la auditoría anterior el oficial enviaba notificaciones y Go no. Ahora los
dos las envían. **Go sigue sin ser más lento en ninguna de las 22
operaciones**:

| Operación | Oficial p50 | Go p50 | Go/oficial (c1 / c2) |
|---|---|---|---|
| Crear | 292 ms | 6,0 ms | 0,02 / 0,02 |
| PATCH | 45,7 ms | 7,4 ms | 0,16 / 0,16 |
| Carga de 64 MiB | 919 ms | 299 ms | 0,33 / 0,33 |
| Descarga de 64 MiB | 90,9 ms | 67,0 ms | 0,74 / 0,70 |
| 64 MiB con 16 clientes | 992 ms (15,3 req/s) | 581 ms (25,1 req/s) | 0,59 / 0,61 |

La CPU y la memoria no cambian respecto de la auditoría sin notificaciones en
Go. Por ejemplo, el ciclo de metadatos gasta 35,3 frente a 1,7 ms de CPU, y Go
ocupa 15–17 MiB de PSS frente a 237–326 MiB del oficial.

La razón de crear no se acercó: con avisos, Go tarda 6,0 ms (antes 5,7). Go
publica fuera de la solicitud, mientras el oficial lo hace dentro de ella.

## Otras verificaciones

`make check` OK (113 pruebas de Python). `glance-test-rbd` 10/10, incluido
`sql-contention`, la prueba nueva del deadlock.

**No es una campaña:** hay pocas repeticiones y el cliente corre en la VM.
