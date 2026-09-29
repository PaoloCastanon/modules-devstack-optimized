# Placement: arquitectura observada en código upstream

**Estado:** análisis de `placement` stable/2026.2 SHA `f4a89d3...` y API oficial; no se ha inspeccionado un proceso DevStack en ejecución. La evidencia de lectura está en [upstream-source-audit.txt](../../evidence/sprint2/placement/upstream-source-audit.txt).

Placement expone HTTP REST para inventarios, capacidad y consumo. Nova Compute (resource tracker) publica resource providers, inventories y traits; Nova Scheduler solicita `GET /allocation_candidates` y reclama recursos con `PUT /allocations/{consumer_uuid}`. Otros consumidores pueden leer capacidad, pero el alcance experimental prioriza Nova. El endpoint se descubre en el catálogo Keystone bajo tipo `placement`; la petición lleva `X-Auth-Token` y `OpenStack-API-Version: placement x.y`. Middleware WSGI de autenticación/política y manejo de microversiones precede al handler; los handlers usan objetos de dominio y SQLAlchemy. Deben verificarse en `placement.conf` y pipeline real tras instalar.

Modelo principal: ResourceProvider (árbol, UUID, generation), Inventory (resource class, total/reserved/ratios), Trait, ResourceClass, Allocation (consumer, provider, clase, used), Consumer (UUID y generation). La DB registra estado y generaciones; conflictos de escritura concurrente devuelven 409. 400 cubre parámetros inválidos; 401/403 autenticación/autorización; 404 recurso ausente. El log de acceso/errores y request ID exactos quedan por comprobar en DevStack.

Secuencia HTTP: catálogo y token Keystone → WSGI auth/microversion/policy → handler → objeto de dominio → DB → serialización JSON/status. La microversión mínima del código inspeccionado es 1.0 y máxima 1.39; eso **no** equivale a haber consultado el servidor. El inventario detallado está en [placement-api-scope.md](placement-api-scope.md). RF02–RF09 se analizan para diseño, sin estado de implementación.

Fuentes: [Placement developer notes](https://docs.openstack.org/placement/latest/contributor/index.html), [API reference](https://docs.openstack.org/api-ref/placement/), [Nova Placement usage](https://docs.openstack.org/placement/latest/user/).
