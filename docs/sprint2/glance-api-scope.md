# Contrato Glance Image API v2 propuesto

Fuente: [API oficial v2](https://docs.openstack.org/api-ref/image/v2/). Autenticación esperada: `X-Auth-Token` Keystone; verificación real pendiente. Baseline disponible: **no**. `Accept: application/json` para metadata; upload binario usa `Content-Type: application/octet-stream`; PATCH usa `application/openstack-images-v2.1-json-patch`. Los status son del contrato documentado y se deben contrastar con la rama instalada.

| Método | Ruta | Auth / headers | Body | HTTP status esperado | Response | Errores | Estado | Persistencia | Requerimiento | Prioridad | Baseline |
|---|---|---|---|---|---|---|---|---|---|---|---|
| POST | `/v2/images` | Token, JSON | name, disk/container format, visibility | 201 | imagen/Location | 400/401/403/409 | queued | DB | RF10 | Must | no |
| GET | `/v2/images` | Token, Accept | filtros/paginación | 200 | `images`, `next` | 401/403 | sin cambio | DB | RF10 | Must | no |
| GET | `/v2/images/{id}` | Token, Accept | — | 200 | metadata | 403/404 | sin cambio | DB | RF10 | Must | no |
| PATCH | `/v2/images/{id}` | Token, JSON Patch | operaciones add/replace/remove | 200 | metadata | 400/403/404/409 | metadata actualizada | DB | RF10 | Must | no |
| DELETE | `/v2/images/{id}` | Token | — | 204 | vacío | 403/404/409 | deleted | DB y store | RF10 | Must | no |
| PUT | `/v2/images/{id}/file` | Token, octet-stream | bytes | 204 | vacío | 400/403/404/409/413 | queued→saving→active/killed | store + DB | RF10 | Must | no |
| GET | `/v2/images/{id}/file` | Token, octet-stream | — | 200 | bytes | 403/404 | sin cambio | store | RF10 | Must | no |

Fuera del alcance inicial: multi-store, import interoperable completo, replicación, federation, cache distribuida y metadefs, salvo que pruebas de Nova muestren dependencia. Los detalles de política, ETag y respuestas de error se fijarán con fixtures reales.
