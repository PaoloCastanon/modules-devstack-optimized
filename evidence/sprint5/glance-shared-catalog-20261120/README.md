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
