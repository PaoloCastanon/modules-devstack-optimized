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

Glance Go usa **el mismo catálogo y los mismos archivos que el Glance
original**; desde el 2026-11-21 es su único modo (el catálogo propio y el store
Ceph RBD se retiraron). Los dos ven las mismas
imágenes: lo que sube uno lo descarga el otro, y lo que borra uno desaparece
en el otro. Al cambiar con `use-go` o `use-original` no se copia nada.

Además, Glance Go **lee el mismo `/etc/glance/glance-api.conf` del original**
(`GLANCE_GO_CONFIG_FILE`) y usa lo mismo que él:

| Pieza | Glance (original) | Glance Go |
|---|---|---|
| Configuración | `/etc/glance/glance-api.conf` | El mismo archivo, leído como oslo.config. `serve.env` solo agrega lo que el archivo no dice: el puerto propio (`127.0.0.1:19292`) y la raíz privada del store |
| Base de datos | `[database] connection` (en DevStack, `root`) | La misma cuenta, en el mismo servidor |
| Catálogo | `images`, `image_properties`, `image_tags` e `image_locations` en `glance` | Las mismas tablas, con la forma de los modelos SQLAlchemy: borrado lógico y una ubicación `file:///…/<id>` por imagen activa |
| Archivos | `filesystem_store_datadir`, cada imagen como `<id>`, escrita directo | Igual: escribe directo en `<id>`, sin copia intermedia ni fsync, y borra el archivo parcial si falla, como `glance_store` |
| Usuario de Keystone | `[keystone_authtoken]` (`glance`, contraseña, proyecto `service`) | El mismo usuario y la misma contraseña, con `token_cache_time` |
| Notificaciones | `[oslo_messaging_notifications]` + `transport_url` | Las mismas |
| Endpoint público | `public_endpoint` | El mismo |

Lo único propio de Glance Go es la tabla `image_data_operations`, que usa para
recuperar cargas interrumpidas. Está en la base `glance_go_shared` del mismo
servidor, porque el original no la tiene. Las imágenes que subió el original
se *adoptan* como cargas confirmadas si su hash es SHA-512 y tienen una sola
ubicación local, la de su ID. Al arrancar, Glance Go verifica
`glance.alembic_version` (`2024_1_contract01`): con otra revisión no arranca.

Como el catálogo es compartido, el original puede borrar o purgar
(`glance-manage db purge`) imágenes que subió Glance Go. Al arrancar, Glance Go
completa esa limpieza en su contabilidad en vez de negarse a servir. La suite
B26 y las campañas también usan este modo.

La base `glance_go` de la instalación anterior con catálogo propio queda en la
VM sin usar; Glance Go ya no la lee.

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
  - la suite B26 se corrió 10 veces con el catálogo propio de entonces;
  - con la misma configuración, la misma cuenta y la escritura directa, se repitió todo: 10/10 y 10/10.
