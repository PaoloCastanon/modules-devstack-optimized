# Modelo Glance

Verificado en `glance/db/sqlalchemy/models.py` SHA `8cd693aadc9d5982d9d71b4c34aab4de937ff70c`; [auditoría](../../evidence/sprint2/glance/upstream-source-audit.txt). La configuración del store de la VM sigue pendiente.

**Metadata en DB:** `images.id` UUID/PK; `name`, `owner`, `status` no nulo, `visibility` (private/public/shared/community), `disk_format`, `container_format`, `size`, `virtual_size`, `checksum`, `os_hash_algo`, `os_hash_value`, `min_disk`, `min_ram`, `protected`, `os_hidden`, `created_at`, `updated_at`, `deleted_at`, `deleted`. `image_properties` tiene PK entera y `image_id` FK, nombre+valor y unique `(image_id,name)`; `image_locations` tiene PK entera, `image_id` FK, URI `value`, `meta_data`, `status`. `image_tags` y `image_members` existen en upstream; el mínimo Go implementará sólo lo exigido por el baseline/Nova.

**Datos binarios en image store:** bytes de imagen administrados por `glance_store`. La DB puede guardar location, tamaño y hash, pero no el payload. En Go se propone un único backend filesystem para el laboratorio con escritura temporal + rename atómico y checksum calculado, sujeto a verificar el backend real. Las transiciones `queued`→`saving`→`active`/`killed` y borrado deben ensayarse antes de fijar persistencia.
