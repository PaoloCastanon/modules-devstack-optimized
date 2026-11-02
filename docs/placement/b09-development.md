# B09 — allocations: incrementos A y cierre

Base B08 6587805 (PR #7 todavía abierto al iniciar). Estado: B09 completado para las rutas priorizadas del perfil el 2026-10-29.
El incremento A añadió la base atómica 1.0–1.8. El cierre añade formato por
provider 1.12, owner en lecturas, consumer generation 1.28 y mappings 1.34,
contrastados con PUT 1.36. El máximo negociable para allocations es 1.36;
default sigue siendo 1.0. Las otras rutas conservan el perfil anterior hasta
B10 y devuelven 406 explícito fuera de él. No se promete toda Placement 1.36.

Contrato fijado a Placement f4a89d3803cb23df22b8714569889ab39c918c34:

- GET /allocations/{consumer}: 200, allocations indexadas por provider y su
  generación actual; un consumer sin reservas devuelve {"allocations":{}}.
- PUT /allocations/{consumer} en 1.0–1.7: array de providers con resources,
  sin project_id/user_id/consumer_generation; respuesta 204. Reemplaza todas
  las reservas del consumer. Providers duplicados: prevalece la última entrada.
- PUT desde 1.8 requiere project_id y user_id (strings no vacíos de hasta 255
  caracteres). Se persisten en la misma transacción que las reservas. GET aún
  no los muestra: ese cambio pertenece a 1.12.
- DELETE /allocations/{consumer}: 204 y eliminación del consumer sin reservas;
  repetir devuelve 404. No incrementa generation de los providers.
- GET /resource_providers/{uuid}/allocations: agrupado por consumer con la
  generación del provider; provider inexistente 404.
- GET /resource_providers/{uuid}/usages: uso agrupado por clase y generation.
- Roles admin/service y scope de proyecto; sin token 401, reader 403.

La nueva migración v4 es append-only y crea consumers/allocations con FK a la
DB propia. El lock del consumer serializa reemplazos/liberaciones; se bloquea
la unión ordenada de providers anteriores y solicitados, en el mismo protocolo
que inventories/traits. Se valida capacidad descontando la reserva anterior del
consumer; si falla una clase/provider o SQL, todo revierte. Un PUT exitoso aumenta
una vez la generación de cada provider solicitado, incluso si se repite el estado.
Los providers que sólo pierden reservas no aumentan su generación en upstream.
El filtro resources descuenta el uso real antes de devolver providers.

El original permite reducir la capacidad del inventario por debajo del uso:
conserva las reservas y registra sobrecapacidad. Se corrige la suposición del
plan: no se inventa un 409 en ese caso. Se bloquean borrados de inventarios y
providers con reservas; no se permiten allocations nuevas que empeoren la
sobrecapacidad. Reemplazar la reserva por otra igual o menor sí está permitido,
siempre que cumpla min/max/step. Se contrasta este comportamiento en vivo.

## Verificación y operación

[Campaña B09-A](../../evidence/sprint3/placement/b09a-20261029/README.md):
**60/60** comparaciones estrictas con MySQL/Keystone reales, B07 **57/57** y
B08 **82/82**. Pasaron las cinco suites MySQL aisladas, el gate make check y
reinicio del ejecutable con reservas persistidas. Los tests de rollback fuerzan
fallo SQL tanto al actualizar el provider como al actualizar el consumer final;
comprueban generaciones, reservas y propietario sin efectos parciales.

La comparación positiva escribe en 1.8 usando project/user ya existentes en la
DB original; las versiones anteriores crearían registros auxiliares globales
que no se pueden limpiar por HTTP. Se conservan fixtures reales de errores y
pruebas locales para el protocolo 1.0–1.7. La lectura SQL posterior confirma que
no quedaron providers/consumers/allocations del ensayo ni cambios de registros
globales. No hubo escritura SQL directa sobre tablas originales.

`make placement-test-allocations` exige schema nuevo vacío, opt-in y archivos
privados de cuentas migrate/serve. El runner `scripts/placement_allocation_differential.py`
exige --allow-mutations, salida nueva, tokens privados y project/user originales
preexistentes. Conserva respuestas crudas y cleanup; sólo normaliza request_id,
origen de Location y tokens Vary como B07. No compara cambiando valores de error,
capacity, tipos, identidades ni generation.

La migración v4 es explícita; serve rechaza v3 e historial dirty/desconocido.
No hay down migration; volver al binario B08 requiere restaurar su DB propia
previa. Las migraciones v1–v3 permanecen intactas. Un consumer se crea o actualiza
en la transacción de reservas; un fallo revierte también esa operación.

La VM quedó apagada y las identidades/secretos del ensayo fueron eliminados.
No se cambia el Excel, capturas S2 ni endpoints de Nova. Se probó un nuevo proceso
y un pool nuevo; no se reinició MySQL ni se simuló caída abrupta.

## Cierre B09-B

Formato indexado implementado desde 1.12; owner se devuelve sólo con reservas.
Desde 1.28 consumer_generation es obligatorio: null exige ausencia del consumer,
zero y otros enteros comparan exactamente con su generación actual. Un conflicto
409 conserva reservas y owner e incluye placement.concurrent_update desde 1.23.
PUT vacío elimina las reservas y el consumer y aumenta los providers anteriores;
DELETE 1.0 conserva la generación de providers. El provider generation opcional
del payload 1.12 no es un CAS del cliente, como el original. Mappings 1.34 se
valida y no se persiste, siguiendo el handler original.

[Cierre B09](../../evidence/sprint3/placement/b09-close-20261029/README.md):
**24/24** casos modernos estrictos, B09-A **60/60**, B07 **57/57**, B08 **82/82**.
Tests SQL añaden carrera de creación, CAS y liberación; make check PASS. La
migración v5 añade created_at a allocations para Last-Modified persistido y
Cache-Control: no-cache desde 1.15. No se reescriben las migraciones anteriores.
POST batch y reshaper quedan fuera de estas rutas priorizadas. El flujo Nova
completo y el boot permanecen en B13–B15; B10 agrega candidates y sus dependencias.
