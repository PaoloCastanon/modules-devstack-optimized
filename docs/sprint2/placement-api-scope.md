# Contrato Placement propuesto

Fuente de mínimos: handlers de Placement stable/2026.2 (`evidence/sprint2/placement/upstream-source-audit.txt`) y [API oficial](https://docs.openstack.org/api-ref/placement/). Servidor/microversión probada: **ninguna**. La rama declara 1.0–1.39; Nova de la rama consultaría candidatos con 1.36 y allocations con 1.28, pendiente de prueba. El subconjunto inicial propuesto cubrirá 1.0, 1.2, 1.6, 1.10, 1.28 y 1.36 según operación; versiones intermedias se deberán negociar/validar antes de cerrar contrato. No se promete cobertura general hasta 1.39.

| Método | Ruta | Función | Microversión mínima | Microversión probada | Request | Response | Errores relevantes | Requerimiento Excel | Prioridad | Alcance | Evidencia baseline |
|---|---|---|---|---|---|---|---|---|---|---|---|
| GET | `/` | Descubrimiento | 1.0 | — | Accept | versions/min/max | 406 | RF02 | Must | Dentro | pendiente |
| GET | `/resource_providers` | Listar | 1.0 | — | filtros | providers | 400/401 | RF03 | Must | Dentro | pendiente |
| POST | `/resource_providers` | Crear | 1.0 | — | UUID, name | provider, 201 | 400/409 | RF03 | Must | Dentro | pendiente |
| GET | `/resource_providers/{uuid}` | Consultar | 1.0 | — | UUID | provider/generation | 404 | RF03 | Must | Dentro | pendiente |
| PUT | `/resource_providers/{uuid}` | Actualizar | 1.0 | — | name/generation | provider | 409/404 | RF03 | Must | Dentro | pendiente |
| DELETE | `/resource_providers/{uuid}` | Eliminar | 1.0 | — | UUID | 204 | 409/404 | RF03 | Must | Dentro | pendiente |
| GET | `/resource_providers/{uuid}/inventories` | Listar | 1.0 | — | UUID | inventories/generation | 404 | RF03 | Must | Dentro | pendiente |
| PUT | `/resource_providers/{uuid}/inventories` | Reemplazar conjunto | 1.0 | — | inventories/generation | generation | 400/409 | RF03 | Must | Dentro | pendiente |
| GET | `/resource_providers/{uuid}/inventories/{resource_class}` | Consultar | 1.0 | — | clase | inventory | 404 | RF03 | Must | Dentro | pendiente |
| GET | `/traits` | Listar | 1.6 | — | filtros | traits | 400 | RF04 | Must | Dentro | pendiente |
| GET | `/resource_providers/{uuid}/traits` | Consultar | 1.6 | — | UUID | traits/generation | 404 | RF04 | Must | Dentro | pendiente |
| PUT | `/resource_providers/{uuid}/traits` | Asociar | 1.6 | — | traits/generation | traits/generation | 409 | RF04 | Must | Dentro | pendiente |
| GET | `/resource_classes` | Listar | 1.2 | — | filtros | classes | 400 | RF04 | Must | Dentro | pendiente |
| GET | `/resource_classes/{name}` | Consultar | 1.2 | — | nombre | class | 404 | RF04 | Must | Dentro | pendiente |
| PUT | `/resource_classes/{name}` | Custom | 1.2 | — | CUSTOM_* | 201/204 | 400/409 | RF04 | Should | Condicional | pendiente |
| GET | `/allocations/{consumer_uuid}` | Consultar | 1.0 | — | UUID | allocations/generation | 404 | RF05 | Must | Dentro | pendiente |
| PUT | `/allocations/{consumer_uuid}` | Reclamar/actualizar | 1.0; 1.28 para consumer generation | — | allocations, project/user, generation | 204 | 400/409 | RF05 | Must | Dentro | pendiente |
| DELETE | `/allocations/{consumer_uuid}` | Liberar | 1.0 | — | UUID | 204 | 404 | RF05 | Must | Dentro | pendiente |
| GET | `/allocation_candidates` | Buscar candidatos | 1.10; 1.36 para same_subtree | — | `resources`, traits, agregados | allocation_requests/provider_summaries | 400 | RF06 | Must | Dentro | pendiente |
| POST | `/reshaper` | Migración de inventario | 1.30 | — | conjuntos | 204 | 409 | RF02 | Could | Fuera inicialmente | pendiente |

El detalle de cuerpos, filtros y códigos puede variar con microversión. Antes de Sprint 3 se debe capturar request/response real para cada operación priorizada y ajustar esta tabla. Una fila «Dentro» significa alcance de diseño, no implementación.
