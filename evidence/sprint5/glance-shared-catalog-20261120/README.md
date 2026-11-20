# Glance Go sobre el catálogo del Glance original: verificación

Fecha 2026-11-20. VM local `tt-openstack-lab` (DevStack, `glance.alembic_version`
`2024_1_contract01`). Rama `feat/glance-official-catalog`. Binario sha256 `ae84bdaf…`.

1. `glance_replace.py install` en modo `official`:
   - copió `mi-cirros` (117 440 512 bytes), que solo estaba en el catálogo propio anterior, al catálogo oficial, con el mismo ID y el mismo dueño;
   - CirrOS ya estaba en los dos;
   - quedó Glance Go sirviendo `/image` y `devstack@g-api` inactivo.
2. `scripts/integration/glance_shared_e2e.py --rounds 10 --network <private>`:
   **10/10 rondas** ([e2e-10-rounds.jsonl](e2e-10-rounds.jsonl)), de unos 27 s cada una. Cada ronda verifica:
   - que Glance Go descarga idéntica la CirrOS que subió el original;
   - que una imagen creada, subida y modificada por Glance Go la ven y la descargan idéntica en el original (metadatos, propiedades, tags y MD5/SHA-512), y viceversa;
   - que lo que borra uno da 404 en el otro y su archivo desaparece de `/opt/stack/data/glance/images`;
   - que Nova crea un servidor `ACTIVE` desde la CirrOS compartida servida por Glance Go.
3. La suite B26, en el modo propio de Glance Go con el original activo, pasó **10/10 corridas**, con 12/12 casos cada una ([b26-own-mode-10-runs.jsonl](b26-own-mode-10-runs.jsonl)).
4. Después de reiniciar la VM, Glance Go arranca solo con `serving the official catalog` y su línea `effective settings`, y una ronda E2E más con Nova pasa.
5. `make check` OK. Las pruebas nuevas cubren:
   - el dialecto SQL del oficial;
   - las cuentas, el store compartido y la configuración;
   - la migración compartida y `effective settings`.

Límites:
- Solo store filesystem. Las imágenes con otro hash o con ubicaciones remotas o múltiples no se adoptan.
- `image_members` sigue fuera del alcance.
- Las pruebas de integración MySQL con contenedor no incluyen todavía el esquema del oficial. La verificación del dialecto es la de la VM, contra el esquema real.

## Misma configuración, misma cuenta y escritura directa (2026-11-20, binario `726f4e90…`)

Glance Go lee el mismo `/etc/glance/glance-api.conf` del original (`GLANCE_GO_CONFIG_FILE`). De ahí toma:
- la misma cuenta MySQL (`[database] connection`);
- el mismo usuario de Keystone (`[keystone_authtoken]`, `glance` con contraseña);
- las notificaciones, `public_endpoint` y `filesystem_store_datadir`.

Escribe cada imagen directo como `<id>`, sin staging ni fsync, y borra el archivo parcial si falla, como `glance_store`.

- E2E compartido con Nova: **10/10 rondas** ([e2e-10-rounds-original-config.jsonl](e2e-10-rounds-original-config.jsonl)).
- B26 en modo propio: **5/5 corridas**, 12/12 casos ([b26-own-mode-5-runs-original-config.jsonl](b26-own-mode-5-runs-original-config.jsonl)). La sexta se detuvo a pedido y el laboratorio quedó restaurado.

## Optimizaciones de la auditoría (binario `54fcb48f…`)

1. **Ver y listar:** imagen, propiedades y tags en una sola consulta, con `JSON_OBJECTAGG`/`JSON_ARRAYAGG` por fila, como el `joinedload` del original. Antes eran 3 consultas.
2. **Descargar:** la fila de la imagen se lee una sola vez por descarga; antes se leía dos veces.
3. **Adopción:** las subidas del original se adoptan una vez al arrancar (`adopted_uploads`), y no en cada descarga.

Medición directa a Glance Go (`127.0.0.1:19292`), con un pedido a la vez: 300 pedidos por caso (100 en la descarga de 21 MiB), tras 30 de calentamiento. Es la mediana de 3 ejecuciones, en ms ([antes](optimization-before.jsonl), [después](optimization-after.jsonl)):

| Caso | Antes | Después | Ahorro |
|---|---:|---:|---:|
| Ver imagen (con 1 propiedad y 2 tags) | 1,391 | 1,290 | −0,10 ms (7 %) |
| Listar (3 imágenes) | 2,217 | 1,654 | −0,56 ms (25 %) |
| Descargar 1 MiB | 1,707 | 1,519 | −0,19 ms (11 %) |
| Ver una imagen subida por el original | 1,477 | 1,340 | −0,14 ms (9 %) |
| Descargar CirrOS, subida por el original (21 MiB) | 12,752 | 12,209 | −0,54 ms (4 %) |

E2E compartido con Nova después de las optimizaciones: **5/5 rondas** ([e2e-5-rounds-optimized.jsonl](e2e-5-rounds-optimized.jsonl)).
