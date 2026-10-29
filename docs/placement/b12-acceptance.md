# B12 — autenticación y políticas del perfil Placement

Fecha: 2026-10-29. Base B11 19854f7. **B12 completado para las rutas y
microversiones priorizadas del laboratorio.** El alcance continúa siendo una
reimplementación parcial para Nova. La sustitución del endpoint y el boot ACTIVE
se verifican en B13–B15.

La [campaña real](../../evidence/sprint3/placement/b12-close-20261029/README.md)
ejecutó el binario normal con MySQL propio y Keystone del laboratorio. Resultado:
**143/143 comparaciones estrictas**, con estado y limpieza verificados:

- 134 casos de políticas: 14 lecturas por identidad Nova/service/admin; escrituras
  de inventario y allocations con Nova; 29 métodos/rutas rechazados por cada
  identidad reader/member/sin proyecto; cabeceras de rol/proyecto/usuario falsas,
  token ausente e inválido.
- Tres consultas con token revocado: 401. Se validó antes en Go para comprobar
  que no se conserva una identidad positiva en caché.
- Tres consultas con token de trust válido: 200; tras su expiración real,
  las mismas tres consultas: 401. Trust delegado service con plazo de 25 segundos.

La identidad Nova se obtuvo de `[placement]` en nova.conf: usuario nova, proyecto
service. Su metadata de roles (incluidos admin y service) se registra sin secretos.
La identidad service aislada también pasó; su validador no tiene admin. Reader y
member incluyen las implicaciones reales de roles de Keystone y siguen denegados.
La política del original usa enforce_new_defaults=True y admin_or_service_api.
Cada rechazo compara el estado antes/después; no se autorizan roles procedentes
de cabeceras. Los tokens de cliente se validan en Keystone en cada solicitud;
sólo se cachea el token del propio validador, acotado a su expiración/proyecto.

Tres verificaciones adicionales exclusivas de Go pasaron: credencial de
validador privilegiada rechazada con 503, endpoint de validación local que
responde 503 rechazado con 503 y X-Service-Token incapaz de promover un reader.
Cada caso conserva el hash de todas las filas; restaurar la credencial válida
recupera el servicio. No se detuvo ni cambió Keystone original. Placement rechaza
la autenticación compuesta dentro de este perfil; no hereda las concesiones de
Glance. Restricciones de token no implementadas se rechazan, sin quitar el bind
ni access_rules. No existe una opción de bypass al desplegar.

## Correcciones contrastadas

El primer intento conservado tuvo 99/134 coincidencias. Los 29 rechazos de tokens
sin proyecto ya eran 403 sin efectos, pero usaban detalle/versión genéricos. Ahora
conservan el error de política y la microversión solicitada, incluido el código de
error desde 1.23. La identidad permanece rechazada antes de acceder al repositorio;
no se modificó auth compartida ni la política de Glance.

Tres GET de inventario individual carecían de Cache-Control/Last-Modified. Ahora
leen la clase y su timestamp persistido en la misma transacción RR que la
generación. La prueba SQL distingue los timestamps de dos clases para impedir
usar el máximo de la colección; el filtro de clase usa parámetros enlazados.
Los otros tres fallos eran expectativas del runner: GET trait individual devuelve
204 en ambos servicios. Se corrigió esa expectativa; no se normalizaron respuestas
incompatibles para ocultar diferencias. Pruebas rojas previas y verdes conservadas.

## Cierre transversal y límites

make check pasó con vet, race detector y build de Placement/Glance. La suite MySQL
real pasó la lectura individual y los snapshots concurrentes. Regresiones vivas:
B07 57/57, B08 82/82, B09-A 60/60, B09 moderno 24/24 y B10 44/44, con seeds iguales.
La campaña B11 se repitió sobre el binario final: nueve verificaciones aprobadas,
incluyendo SIGKILL Go/MySQL, rollback InnoDB y restore byte a byte de todas las filas.

La limpieza HTTP y SQL de sólo lectura confirmó cero providers, consumers y
allocations propios en el original; sus listas de projects/users no cambiaron.
Se eliminaron identidades/application credentials/trusts temporales, se revocó
el token emitido para Nova y se eliminaron secretos y contenedores/volúmenes de
prueba. La VM se apagó al terminar. No se cambiaron Nova, endpoints,
Excel ni capturas históricas de S2; no se ejecutó SQL de escritura en la DB original.

La revocación no acredita el comportamiento de la caché del middleware Python:
el original no había visto ese token antes de revocarlo. No se promete federación,
system scope, autenticación compuesta, políticas personalizadas, árboles/sharing,
HA ni la API completa. La indisponibilidad se inyectó sólo en el cliente Go local.
B13–B15 siguen pendientes y requieren sustitución reversible y boot con Nova.
