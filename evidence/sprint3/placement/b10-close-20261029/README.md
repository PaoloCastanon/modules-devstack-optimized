# Cierre B10 — candidates del perfil plano, 2026-10-29

Base 0e59cf3, cierre B09. Original Placement fijado f4a89d3; Nova inspeccionado
247ea5d. La implementación es parcial: providers planos del laboratorio y las
versiones concretas del tracker. No se acredita árboles/recursos compartidos,
POST batch, reshaper, toda API 1.36 ni sustitución/boot Nova.

## Resultado observado

- live/b10-run/report.json: **44/44** status, cabeceras estables, JSON y estados
  coinciden; ocho respuestas de seed sin diferencias. CUSTOM class nueva acota
  las consultas para excluir providers preexistentes del original. Casos:
  representaciones 1.10/1.12/1.17/1.27/1.29/1.34, root_required del cliente Nova,
  limit, claims/uso real, forbidden, member_of, grupos/policy/same_subtree, errores,
  reader/401 y perfiles providers 1.14/1.20, aggregates 1.19, inventory 1.26.
- Regresiones: B09 moderno **24/24**, B09-A **60/60**, B07 **57/57**, B08 **82/82**.
- candidates-mysql/: MySQL real, estado persistido y asociaciones positivas de
  aggregate, CAS de aggregate y snapshot RR durante 40 reemplazos concurrentes.
  Una relación generation/usage alternante comprueba que no se mezclan estados.
- regression-mysql/: las cinco suites B09/B08/B07, lifecycle binario y colisión
  de upgrade pasan en schemas nuevos separados y cuentas migrate/serve propias.
- make-check.log: formato/vet/race/build de ambos módulos, harness, Python,
  documentos e infraestructura PASS. route-red.log conserva el 406 inicial de
  la ruta antes de implementar su perfil; el test ahora pasa con 200 vacío.
- live/restart-proof.json: reservas sobreviven a SIGTERM y nuevo proceso.

Las diferencias temporales de Last-Modified se normalizan sólo tras validar
HTTP date, además de request_id/origen/Vary. Se ordenan allocation_requests y
traits de summaries sin deduplicar. No se transforman capacidad, used, tipos,
status, códigos ni generaciones. Datos crudos completos conservados.

## Implementación y procedencia

Una transacción repeatable-read obtiene providers, clases, inventarios, usos,
traits y aggregates; el servicio deriva candidatos y summaries. Las reservas
reducen disponibilidad y las sumas de grupos no exceden capacidad/max_unit.
El perfil plano evalúa in_tree y same_subtree como raíces sin hijos y rechaza
un parent no nulo o estado compartido, en vez de ignorarlo. group_policy=isolate
con dos grupos numerados no tiene candidato en un árbol plano.

Los máximos por ruta se documentan en docs/placement/b10-development.md. Las
versiones más altas en otra ruta devuelven 406 explícito. La migración v6 añade
tiempos persistidos de provider/inventory; v1–v5 no se modifican. Para filas
históricas sin timestamps se usa el instante de migración, no una fecha inventada.
source-files.sha256 identifica fuentes e inputs; live/provenance.json registra
binario, toolchain, MySQL 8.0.46 y Keystone desplegado.

original-cleanup.json confirma 0 providers/consumers/allocations propios y mismos
registros globales. Cleanup HTTP incluye la clase/trait CUSTOM, con GET ausentes.
No hubo SQL de escritura al original. La identidad temporal y VM permanecen en
uso para el cierre B11/B12 autorizado; la limpieza final se registra allí.

## Reproducción

Usar un schema nuevo vacío por suite, cuentas propias separadas, los dos DSN
files privados y PLACEMENT_GO_MYSQL_TEST=1 como B09. Ejecutar
make placement-test-candidates y las cinco suites previas; make check es local.
Para HTTP, Go con su MySQL propio y Keystone real, URL pública igual al mount:

```sh
python3 scripts/placement_candidates_differential.py \
  --python-url http://192.168.122.10/placement --go-url "$GO_PLACEMENT_URL" \
  --project-id "$EXISTING_PLACEMENT_PROJECT_ID" --user-id "$EXISTING_PLACEMENT_USER_ID" \
  --admin-token-file "$ADMIN_TOKEN_FILE" --service-token-file "$SERVICE_TOKEN_FILE" \
  --reader-token-file "$READER_TOKEN_FILE" --output "$NEW_B10_OUTPUT" --allow-mutations
```

Los project/user deben existir ya en Placement. Repetir runners anteriores en
otras carpetas nuevas. En el original sólo se escribe [] para aggregates; su
asociación positiva se prueba en MySQL propio para no dejar filas globales sin
DELETE HTTP. No cambiar Excel/S2/endpoints; eliminar identidad y apagar VM al
terminar el cierre transversal.
