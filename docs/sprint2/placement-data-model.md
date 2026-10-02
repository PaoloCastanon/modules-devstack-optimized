# Modelo Placement

Verificado contra `placement/db/sqlalchemy/models.py` SHA desplegado `f4a89d3803cb23df22b8714569889ab39c918c34`; [evidencia de inspección](../../evidence/sprint2/placement/upstream-source-audit.txt). Se comprobó la existencia de las tablas `resource_providers`, `inventories`, `allocations` y `consumers` en la [DB desplegada](../../evidence/sprint2/environment/database-schema-audit.txt); los detalles de columnas y constraints proceden del modelo upstream.

| Tabla / entidad | PK | Relación / campos críticos | Restricción observada |
|---|---|---|---|
| `resource_providers` | `id` entero | `uuid`, `name`, `generation`, `root_provider_id`, `parent_provider_id` | UUID y nombre únicos; parent/root FK autorreferente |
| `inventories` | `id` | `resource_provider_id`, `resource_class_id`, total, reserved, min/max/step, allocation_ratio | provider+class único; relaciones ORM por IDs, **sin FK SQL declarada en modelo** |
| `resource_classes` | `id` | `name` | nombre único |
| `traits` | `id` | `name` | nombre único |
| `resource_provider_traits` | provider_id+trait_id | FK a traits; provider relation | PK compuesta; provider FK declarada |
| `allocations` | `id` | `resource_provider_id`, `consumer_id` (UUID string), `resource_class_id`, `used` | índices; **sin FK SQL declarada en modelo** |
| `consumers` | `id` | `uuid`, `project_id`, `user_id`, `generation`, `consumer_type_id` | UUID único; FK a consumer_types |

La generation de provider y consumer es control de concurrencia: update con generation obsoleta puede devolver 409. Los vínculos lógicos no equivalen siempre a constraints SQL; distinguirlos impide diseñar un schema Go incorrecto. Subconjunto Go: providers, inventory, classes, traits, consumers y allocations con transacciones y comparaciones de generation; evaluar árboles y agregados según baseline real.
