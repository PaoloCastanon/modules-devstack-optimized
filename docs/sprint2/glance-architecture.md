# Glance: arquitectura observada en upstream

Glance API v2 ofrece catálogo de metadata e image data. Keystone autentica con token y el catálogo expone tipo `image`; política controla autorización. Las capas documentadas son API REST → dominio/controlador → DB de metadata y `glance_store` para bytes. `Image` mantiene UUID, owner, status, visibility, formatos, tamaño, hashes, fechas y flags; `ImageProperty`, `ImageLocation` y otras tablas se relacionan con ella. El binario vive en el store, no en `images`. [Auditoría del modelo](../../evidence/sprint2/glance/upstream-source-audit.txt).

Flujo propuesto: `POST /v2/images` reserva metadata (`queued`), `PUT /v2/images/{id}/file` transmite bytes (`saving`→`active` o `killed`), `GET /v2/images/{id}/file` entrega bytes; Nova consulta metadata y usa la imagen para el arranque. El backend específico, ruta de filesystem, pipeline auth, políticas y estados efectivos de **este laboratorio** no están disponibles hasta instalar DevStack y leer `glance-api.conf`. No se asume una ruta de store.

Fuentes: [Glance architecture](https://docs.openstack.org/glance/latest/contributor/architecture.html), [Image API v2](https://docs.openstack.org/api-ref/image/v2/), [Nova architecture](https://docs.openstack.org/nova/latest/admin/architecture).
