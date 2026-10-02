# Contrato Placement propuesto

Fuente de mínimos: handlers de Placement stable/2026.2 (`evidence/sprint2/placement/upstream-source-audit.txt`) y [API oficial](https://docs.openstack.org/api-ref/placement/). El servidor desplegado anuncia microversiones **1.0–1.39** en [`versions.response.json`](../../evidence/sprint2/placement/versions.response.json). El baseline probó operaciones con 1.0, 1.2, 1.6 y 1.10, incluidos create, update y delete de un provider temporal, inventario y error 404. La [traza de Placement](../../evidence/sprint2/placement/nova-requests.log) confirma que Nova usó 1.36 para buscar candidatos y hacer `PUT /allocations`, 1.28 para `GET /allocations` y 1.0 para `DELETE /allocations` durante el boot y la limpieza. El subconjunto inicial propuesto cubre 1.0, 1.2, 1.6, 1.10, 1.28 y 1.36 según operación. No se promete cobertura general hasta 1.39.

| Método | Ruta | Función | Microversión mínima | Microversión probada | Request | Response | Errores relevantes | Requerimiento Excel | Prioridad | Alcance | Evidencia baseline |
|---|---|---|---|---|---|---|---|---|---|---|---|
| GET | `/` | Descubrimiento | 1.0 | 1.0–1.39 anunciado | Accept | versions/min/max | 406 | RF02 | Must | Dentro | versions |
| GET | `/resource_providers` | Listar | 1.0 | 1.0 | filtros | providers | 400/401 | RF03 | Must | Dentro | providers |
| POST | `/resource_providers` | Crear | 1.0 | 1.0 | UUID, name | provider, 201 | 400/409 | RF03 | Must | Dentro | provider-create |
| GET | `/resource_providers/{uuid}` | Consultar | 1.0 | 1.0 | UUID | provider/generation | 404 | RF03 | Must | Dentro | provider |
| PUT | `/resource_providers/{uuid}` | Actualizar | 1.0 | — | name/generation | provider | 409/404 | RF03 | Must | Dentro | pendiente |
| DELETE | `/resource_providers/{uuid}` | Eliminar | 1.0 | 1.0 | UUID | 204 | 409/404 | RF03 | Must | Dentro | provider-delete |
| GET | `/resource_providers/{uuid}/inventories` | Listar | 1.0 | 1.0 | UUID | inventories/generation | 404 | RF03 | Must | Dentro | inventories |
| PUT | `/resource_providers/{uuid}/inventories` | Reemplazar conjunto | 1.0 | 1.0 | inventories/generation | generation | 400/409 | RF03 | Must | Dentro | inventory-put |
| GET | `/resource_providers/{uuid}/inventories/{resource_class}` | Consultar | 1.0 | 1.0 | clase | inventory | 404 | RF03 | Must | Dentro | inventory-created |
| GET | `/traits` | Listar | 1.6 | — | filtros | traits | 400 | RF04 | Must | Dentro | pendiente |
| GET | `/resource_providers/{uuid}/traits` | Consultar | 1.6 | 1.6 | UUID | traits/generation | 404 | RF04 | Must | Dentro | traits |
| PUT | `/resource_providers/{uuid}/traits` | Asociar | 1.6 | — | traits/generation | traits/generation | 409 | RF04 | Must | Dentro | pendiente |
| GET | `/resource_classes` | Listar | 1.2 | 1.2 | filtros | classes | 400 | RF04 | Must | Dentro | classes |
| GET | `/resource_classes/{name}` | Consultar | 1.2 | — | nombre | class | 404 | RF04 | Must | Dentro | pendiente |
| PUT | `/resource_classes/{name}` | Custom | 1.2 | — | CUSTOM_* | 201/204 | 400/409 | RF04 | Should | Condicional | pendiente |
| GET | `/allocations/{consumer_uuid}` | Consultar | 1.0 | 1.28 (Nova) | UUID | allocations/generation | 404 | RF05 | Must | Dentro | nova-requests.log |
| PUT | `/allocations/{consumer_uuid}` | Reclamar/actualizar | 1.0; 1.28 para consumer generation | 1.36 (Nova) | allocations, project/user, generation | 204 | 400/409 | RF05 | Must | Dentro | nova-requests.log |
| DELETE | `/allocations/{consumer_uuid}` | Liberar | 1.0 | 1.0 (Nova) | UUID | 204 | 404 | RF05 | Must | Dentro | nova-requests.log |
| GET | `/allocation_candidates` | Buscar candidatos | 1.10; 1.36 para same_subtree | 1.10; 1.36 (Nova) | `resources`, traits, agregados | allocation_requests/provider_summaries | 400 | RF06 | Must | Dentro | candidates; nova-requests.log |
| POST | `/reshaper` | Migración de inventario | 1.30 | — | conjuntos | 204 | 409 | RF02 | Could | Fuera inicialmente | pendiente |

Los archivos `evidence/sprint2/placement/{versions,providers,provider-create,provider-created,inventory-put,inventory-created,provider-delete,classes,traits,candidates,provider-not-found}.*` contienen request, response y status de las operaciones probadas. Las filas con «—» en la columna de microversión probada siguen como alcance de diseño y requieren fixtures específicos en Sprint 3; «Dentro» no significa implementación Go.
