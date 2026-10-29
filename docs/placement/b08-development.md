# B08 — catálogos, traits y compatibilidad de versiones

Base: main `b4d8300`, que integra B07 mediante PR #6. Estado: implementado y validado para el alcance seleccionado el 2026-10-29.

El criterio del incremento es administrar resource classes/traits estándar y
CUSTOM, asociar traits a providers con generation y proteger catálogos en uso.
Se preservan las operaciones B07, su estado y sus errores 1.0.

## Contrato implementado

| Versión | Cambio seleccionado y prueba observable |
|---|---|
| 1.1 | GET/PUT aggregates por provider; body de PUT es array de UUID, sin generation en respuesta ni incremento |
| 1.2 | GET/POST resource_classes, GET/PUT/DELETE por nombre; POST crea, PUT renombra CUSTOM con body JSON hasta 1.6 |
| 1.3 | member_of en listado de providers, con UUID o in:UUID,... |
| 1.4 | resources en listado de providers, con capacidad/min/max/step del inventario persistido |
| 1.5 | DELETE del conjunto de inventories; incremento de generation y conjunto vacío |
| 1.6 | Catálogo traits, PUT idempotente CUSTOM, GET 204, DELETE protegido y GET/PUT/DELETE traits por provider |
| 1.7 | PUT resource_classes/{CUSTOM} sin body: 201 al crear, 204 si existe; sustituye al PUT de renombre |

Estas dependencias impiden aumentar el máximo directamente de 1.0 a 1.6 sin
implementar los cambios intermedios relevantes de las rutas seleccionadas.
Las rutas nuevas devuelven 404 antes de su versión mínima; DELETE inventories
devuelve 405 antes de 1.5. No se promete toda la API de esas microversiones.

Los catálogos estándar se fijan a los paquetes del laboratorio: os-resource-classes
1.1.0 (21 nombres) y os-traits 3.9.0 (377 nombres). Se incorporan mediante una
migración append-only v3: v1/v2 y sus checksums permanecen intactos. No se copia
la DB original ni se consulta Python para resolver solicitudes Go.

Un PUT de traits con generation vieja falla sin efectos. Un conjunto idéntico
no incrementa generation; cambiarlo o vaciarlo sí. Las asociaciones y modificaciones
de inventories se serializan con el mismo lock del provider. Eliminar CUSTOM
en uso devuelve conflicto; no se permite modificar/eliminar nombres estándar.
Renombrar una clase CUSTOM mantiene el inventario y la generación del provider.

## Verificación observada

[Campaña B08](../../evidence/sprint3/placement/b08-close-20261029/README.md):
**82/82** respuestas y estados estrictos, con Keystone/MySQL reales; regresión
B07 **57/57** en 1.0. Pasaron make check, upgrade v2→v3 con canary B07, migración
idempotente, catálogos estándar completos, protección de elementos en uso,
rollback tras error SQL y CAS concurrente traits/inventories. El reinicio del
proceso conserva clase CUSTOM, inventario, traits, asociaciones y generation.
No se reinició MySQL. VM apagada e identidades de ensayo eliminadas.

Las listas de traits/aggregates y clases se comparan como conjuntos sin orden,
sin deduplicar. Sólo los catálogos globales excluyen CUSTOM ajenos al ensayo;
se conservan respuestas crudas y se comparan todos los estándar y CUSTOM propios.
Errores, tipos, generation y efectos no se normalizan. Los ocho errores Python
capturados tienen tests de regresión exactos. Las diferencias de los ensayos
iniciales y el error de DDL están conservados en la evidencia.

La asociación positiva de aggregates se verifica en MySQL aislado. Sobre el
original sólo se escribe []; no se crean filas globales imposibles de limpiar
por HTTP. No se atribuye cobertura diferencial viva a esa asociación positiva.

## Operación

La migración v3 es explícita con la cuenta migrate; serve exige historial completo
y usa su cuenta DML. No se permite arrancar sobre v2, ni sobre un historial dirty,
modificado o desconocido. Si falla DDL, revisar manualmente la DB propia; no borrar
el guard ni repetir sobre dirty. Los checksums de v1/v2 permanecen intactos.
El rollback binario a B07 requiere restaurar su DB propia desde backup porque
B07 rechaza una migración futura. No hay down migration automática.

`make placement-test-catalogs` requiere una DB nueva vacía, opt-in y archivos
privados de cuentas separadas como las otras suites MySQL. El runner
`scripts/placement_catalog_differential.py` requiere --allow-mutations, salida
nueva y token files privados; compara solicitudes y estados antes/después y
limpia exclusivamente recursos con GET 404 previo. Ver reproducción en evidencia.
Default sigue siendo 1.0 y latest negocia 1.7; versiones superiores devuelven 406.
El rango se limita a las rutas seleccionadas de la tabla, no a toda Placement.

La prueba de resources parte de que esta DB Go aún no admite allocations. B09
deberá añadir el uso reservado al cálculo antes de habilitar reservas. No se
declara candidates, sustitución Nova, benchmarks ni cierre transversal B11/B12.
No se cambia el Excel ni se sobrescriben capturas S2. La VM se usó para inspección/comparación y quedó apagada al terminar.
