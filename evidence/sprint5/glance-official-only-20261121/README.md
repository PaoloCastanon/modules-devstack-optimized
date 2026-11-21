# Glance Go solo en modo oficial (2026-11-21)

Revisión `5422f4c` (rama `feat/glance-official-only`). Glance Go deja de tener
catálogo propio y store Ceph RBD: siempre lee el `glance-api.conf` del original
y sirve su base `glance`, su cuenta MySQL, su `filesystem_store_datadir` y su
usuario de Keystone. Binario estático `c2c7c2e9ee5354bcce74199a1ec136fca14fffc7e134085e60db7ae83a6ea9e4`
(se recompiló desde el commit con el mismo hash).

## Resultados

| Verificación | Resultado |
| --- | --- |
| Pruebas Go (`go test -race` de Glance, config y auth; `go vet`; `go test` de todo el repo) | OK |
| Gate sin `go-check` de Placement (`docs`, `evidence`, `harness`, `unit-test`, `nova-pin`, `infra`) | OK |
| Integración MySQL contra el DDL real del original, con `-race` ([integration/](integration/)) | 9/9 suites |
| B26, Glance Go ([b26/](b26/)) | 10/10 corridas, 12/12 casos cada una |
| B26 conjunta con Placement Go | 2/2 corridas, 16/16 casos |
| E2E compartido original ↔ Go + Nova ([shared-e2e/](shared-e2e/shared-e2e.log)) | 10/10 rondas |
| Reemplazo permanente reinstalado (`glance_replace.py install`) | `glance-go.service` activo, `/image` → Glance Go |
| Purga real del original y arranque de Glance Go | 5 402 imágenes borradas purgadas (`glance-manage db purge` y `purge_images_table`, `--age_in_days 0`); al arrancar: 78 operaciones revisadas, 76 huérfanas olvidadas, 2 conservadas; 0 huérfanas después |

Todas las corridas de B26 terminaron con `pending_images: 0`, catálogo y sitios
del original sin cambios y los controles de mantenimiento restaurados.

## Hallazgos corregidos con estas pruebas

- **PATCH con 409 sobre el esquema real.** Las columnas `DATETIME` del original
  guardan segundos. Un PATCH que solo cambia propiedades o tags dentro del mismo
  segundo deja la fila `images` igual, y MySQL informaba 0 filas *cambiadas*.
  Ahora la conexión cuenta filas *encontradas* (`CLIENT_FOUND_ROWS`), como los
  dialectos MySQL de SQLAlchemy que usa el original. Lo detectaron las suites
  `catalog` y `contention` en la primera corrida de integración.
- **Arranque tras una purga del original.** La recuperación fallaba si
  `glance-manage db purge` había borrado filas de imágenes con contabilidad en
  `glance_go_shared`, o si el original había borrado una imagen subida por Go.
  Ahora completa la limpieza (borrado del original) u olvida la fila huérfana
  (purga), con pruebas unitarias e integración.
- **B26** exigía que los bytes de la imagen de Nova *no* estuvieran en el store
  del original (supuesto del modo propio). Ahora exige que estén como `<id>` con
  los mismos hashes.

## Cómo reproducir

```sh
make glance-test-integration                       # VM encendida; con -race:
GLANCE_INTEGRATION_RACE=1 make glance-test-integration
# En la VM (ver docs/glance/b26-integration-suite.md):
python3 scripts/integration/glance_suite.py --binary <glance> --flavor-id 42 \
  --network-id <private> --work-root /home/stack/tt-placement-b26-<ts> \
  --output /home/stack/b26-out-<ts> --allow-mutations [--with-placement-go --placement-binary <placement>]
python3 scripts/integration/glance_replace.py install --binary <glance> --allow-mutations
python3 scripts/integration/glance_shared_e2e.py --rounds 10 --network <private> --allow-mutations
```

## Límites

- La integración usa el MySQL de la VM (MySQL 8.0 de DevStack) y esquemas de
  prueba que el script borra al salir; `glance` solo se lee.
- `versions.txt` marca `git_dirty=1` por la carpeta de evidencia v6 sin
  commitear, ajena a este cambio.
- La base `glance_go` de la instalación vieja con catálogo propio sigue en la VM,
  sin uso.
