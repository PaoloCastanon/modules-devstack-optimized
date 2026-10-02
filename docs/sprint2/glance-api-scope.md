# Contrato Glance Image API v2 propuesto

Fuente: [API oficial v2](https://docs.openstack.org/api-ref/image/v2/). El servidor desplegado anuncia Image API v2.18 y el baseline ejercitó create, list, show, PATCH, upload, download y delete. La petición sin token a `/v2/images` devolvió 401; las peticiones autenticadas con token Keystone obtuvieron las respuestas esperadas. El store local usa `/opt/stack/data/glance/images/`, verificado en [`backend-audit.txt`](../../evidence/sprint2/glance/backend-audit.txt). `Accept: application/json` sirve para metadata; upload binario usa `Content-Type: application/octet-stream`; PATCH usa `application/openstack-images-v2.1-json-patch`. El download coincidió byte a byte y por SHA256 con el upload. Los archivos `evidence/sprint2/glance/image-*` conservan requests, respuestas, headers saneados y status.

| Método | Ruta | Auth / headers | Body | HTTP status esperado | Response | Errores | Estado | Persistencia | Requerimiento | Prioridad | Baseline |
|---|---|---|---|---|---|---|---|---|---|---|---|
| POST | `/v2/images` | Token, JSON | name, disk/container format, visibility | 201 | imagen/Location | 400/401/403/409 | queued | DB | RF10 | Must | image-create |
| GET | `/v2/images` | Token, Accept | filtros/paginación | 200 | `images`, `next` | 401/403 | sin cambio | DB | RF10 | Must | images |
| GET | `/v2/images/{id}` | Token, Accept | — | 200 | metadata | 403/404 | sin cambio | DB | RF10 | Must | image |
| PATCH | `/v2/images/{id}` | Token, JSON Patch | operaciones add/replace/remove | 200 | metadata | 400/403/404/409 | metadata actualizada | DB | RF10 | Must | image-patch |
| DELETE | `/v2/images/{id}` | Token | — | 204 | vacío | 403/404/409 | deleted | DB y store | RF10 | Must | image-delete |
| PUT | `/v2/images/{id}/file` | Token, octet-stream | bytes | 204 | vacío | 400/403/404/409/413 | queued→saving→active/killed | store + DB | RF10 | Must | image-upload |
| GET | `/v2/images/{id}/file` | Token, octet-stream | — | 200 | bytes | 403/404 | sin cambio | store | RF10 | Must | image-download |

Fuera del alcance inicial: multi-store, import interoperable completo, replicación, federation, cache distribuida y metadefs, salvo que pruebas de Nova muestren dependencia. El smoke test arrancó una instancia desde la imagen CirrOS de Glance, pero no captura el intercambio HTTP Nova→Glance. Los detalles de política, ETag y errores adicionales requieren fixtures de Sprint 3.
