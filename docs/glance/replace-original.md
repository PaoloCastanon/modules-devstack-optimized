# Reemplazo permanente del Glance original por Glance Go

`scripts/integration/glance_replace.py` deja Glance Go como el servicio de
imágenes de la VM del laboratorio, de forma permanente. Sobrevive a los
reinicios y Nova, Horizon y el cliente `openstack` lo usan sin ningún cambio.
El Glance original no se borra: queda detenido y deshabilitado, y se puede
volver a él con un comando.

## Uso (dentro de la VM, con `openrc admin admin` cargado)

```sh
python3 scripts/integration/glance_replace.py install --binary bin/glance --allow-mutations
python3 scripts/integration/glance_replace.py status
python3 scripts/integration/glance_replace.py use-original --allow-mutations   # volver al oficial
python3 scripts/integration/glance_replace.py use-go --allow-mutations         # volver a Glance Go
```

Antes de correr la suite B26 o una campaña, ejecuta `use-original`, porque las
dos parten del oficial. Si no lo haces, fallan cerradas al comprobar el sitio
de Apache.

## Qué hace `install`

Desde el 2026-11-20, Glance Go usa **el mismo catálogo y los mismos archivos
que el Glance original** (`GLANCE_GO_CATALOG=official`). Los dos ven las mismas
imágenes: lo que sube uno lo descarga el otro, y lo que borra uno desaparece
en el otro. Al cambiar con `use-go` o `use-original` no se copia nada.

| Pieza | Permanente |
|---|---|
| Catálogo | Las tablas del oficial en la base `glance`: `images`, `image_properties`, `image_tags` e `image_locations`. Glance Go escribe con la forma que usan los modelos SQLAlchemy del original: hijos con `created_at`/`deleted`, una fila en `image_locations` `file:///…/<id>` por cada imagen activa y borrado lógico de las ubicaciones, propiedades y tags al borrar la imagen |
| Archivos | El directorio `filesystem_store_datadir` del oficial (`/opt/stack/data/glance/images`), con el nombre `<id>` que usa `glance_store`. La escritura temporal es privada (`/opt/stack/data/glance-go/shared-store`) y se publica con un `link` atómico en el mismo sistema de archivos |
| Contabilidad propia | `image_data_operations` (recuperación de cargas) en la base `glance_go_shared`, sin FK hacia `images`. Las imágenes que subió el original se *adoptan* como cargas confirmadas si su metadato es SHA-512 y tienen una sola ubicación local, la de su ID |
| Cuentas MySQL | `glance_go_shared_migrate`: DDL solo en `glance_go_shared`. `glance_go_shared_app`: DML en `glance_go_shared` y en `glance`, nunca DDL sobre el esquema del original. Glance Go lo verifica al arrancar |
| Revisión del original | Al arrancar se comprueba `glance.alembic_version` (`2024_1_contract01`). Con otra revisión, Glance Go no arranca |
| Credencial | Usuario `glance-go` en Keystone, con el rol `service` en el proyecto `service` y una credencial de aplicación |
| Servicio | `glance-go.service` (systemd, habilitado), en `127.0.0.1:19292` |
| Notificaciones | Las mismas del oficial (`messagingv2`) |
| Ruta | `/image` en Apache apunta a Glance Go; `devstack@g-api` queda deshabilitado, sin borrarlo |

Si antes había una instalación con catálogo propio (base `glance_go`), las
imágenes que solo tenía ella se copian primero al catálogo oficial, con el
mismo ID, el mismo dueño y los hashes verificados. Los datos viejos quedan
sin usar y no se borran.

El modo propio (`GLANCE_GO_CATALOG=own`, por defecto) sigue igual: lo usan la
suite B26 y las campañas, con su propia base y su propio store.

**Límites:** el catálogo compartido solo admite el store filesystem. Las
imágenes del original con otro algoritmo de hash, o con ubicaciones remotas o
múltiples, no se adoptan: Glance Go las lista, pero no las descarga.
`image_members` no se usa, porque las imágenes compartidas siguen fuera del
alcance.

## Verificación (2026-11-20)

- [Reemplazo inicial](../../evidence/sprint5/glance-replace-20261120/README.md):
  - CirrOS se copió;
  - Nova levantó un servidor `ACTIVE`;
  - el cambio funciona en los dos sentidos y sobrevive al reinicio.
- [Catálogo compartido](../../evidence/sprint5/glance-shared-catalog-20261120/README.md):
  - `scripts/integration/glance_shared_e2e.py` pasó **10/10 rondas**;
  - la suite B26 se corrió 10 veces en modo propio.
