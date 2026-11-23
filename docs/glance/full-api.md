# Glance Go: API completa sobre el Glance original

Además de las 7 operaciones que usa Nova (crear, listar, ver, editar, borrar,
subir y bajar), Glance Go implementa el resto de la API v2 que DevStack deja
activa. Todo funciona sobre las mismas tablas, archivos y `glance-api.conf`
del original.

| Parte | Rutas | Tablas del original | Configuración que lee |
|---|---|---|---|
| Miembros | `/v2/images/{id}/members[/{member}]` | `image_members` | — |
| Tags por ruta | `PUT/DELETE /v2/images/{id}/tags/{tag}` | `image_tags` | — |
| Desactivar / reactivar | `POST /v2/images/{id}/actions/{deactivate,reactivate}` | `images` | — |
| Cuotas de Keystone | `/v2/info/usage`; límites al crear y subir | uso desde `images`, `image_locations`, `image_properties` | `use_keystone_limits`, `[oslo_limit]` |
| Stores | `/v2/info/stores[/detail]` (un store: 404 como el original) | — | `enabled_backends` (ausente) |
| Importación interoperable | `PUT /stage`, `POST /import`, `/v2/info/import` | `images`, `image_properties` (`os_glance_import_task`) | `node_staging_uri`, `enabled_import_methods`, `[import_filtering_opts]` |
| Tareas | `/v2/tasks`, `/v2/tasks/{id}`, `/v2/images/{id}/tasks` | `tasks`, `task_info` | `[task] task_time_to_live` |
| Caché de imágenes | `/v2/cache[/{image_id}]`; descargas servidas desde la caché | `cached_images`, `node_reference` | `[paste_deploy] flavor`, `image_cache_dir`, `worker_self_reference_url`, `cache_prefetcher_interval` |
| Metadefs | `/v2/metadefs/namespaces/...`, `/v2/metadefs/resource_types` | `metadef_*` (6 tablas) | — |
| Schemas | `/v2/schemas/*` (16 documentos) | — | — |

## Cómo se verifica

- `make check`: pruebas unitarias con `-race` de cada capa.
- `make glance-test-integration`: 11 suites sobre una copia del DDL real.
  `members` cubre miembros, tags, acciones y uso. `imports` cubre importación,
  tareas, recuperación y caché de punta a punta por HTTP.
- `scripts/integration/glance_differential.py`: la misma secuencia contra el
  original vivo y contra Glance Go, con las respuestas normalizadas.
  Resultado del 2026-11-23: 449 de 450 casos idénticos
  (`evidence/sprint5/glance-full-api-20261123/differential.json`).

## Diferencias declaradas

- Un 400 de validación de metadefs no incluye el volcado de `jsonschema` que
  el original agrega después de la primera línea.
- La importación escribe los datos por el mismo camino que una subida:
  durante la escritura la imagen se ve `saving` (el original la muestra
  `importing` hasta quedar `active`).
- La caché copia una descarga completa en segundo plano desde el store (el
  original la copia mientras responde). El resultado es el mismo, y la primera
  descarga conserva `sendfile`.

## Lo que no implementa todavía

- Varios stores (`enabled_backends`), `copy-image` y `glance-download`.
- La API de ubicaciones (`/v2/images/{id}/locations`, `show_multiple_locations`).
- La inspección del formato al subir (`[image_format] require_image_format_match`,
  activa en DevStack).
- Protección de propiedades (`property_protection_file`), plugins de
  importación (`image_import_plugins`), borrado diferido y `glance-scrubber`.
- La tarea legada `POST /v2/tasks` (tipo `import`), y las herramientas
  `glance-manage` y `glance-cache-*`, que siguen siendo del original.
