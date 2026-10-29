# Cierre B09 — allocations del perfil Nova

Base B09-A 8fa51ae. Contrato fijado a Placement f4a89d3803cb23df22b8714569889ab39c918c34.
El incremento completa el formato indexado por provider (1.12), owner en lecturas,
consumer generation (1.28), liberación vacía y mappings (1.34) para PUT 1.36.
No incluye POST batch /allocations, reshaper ni integración Nova B13–B15.

## Resultado

- live/b09-run/report.json: **24/24** comparaciones estrictas modernas, con
  MySQL/Keystone reales. Cubre null/ausente/boolean/zero/stale/futuro, creación,
  propietario en GET 1.12, generación en GET 1.28 y por provider, provider generation
  ignorada en payload, mappings, rollback de capacidad, actualización legacy,
  liberación vacía con CAS, recreación y DELETE 1.0/repetido.
- Regresiones exactas: B09-A **60/60**, B07 **57/57**, B08 **82/82**.
- mysql/: cinco suites reales con schemas nuevos y cuentas separadas. La prueba
  B09 añade CAS y carrera de creación del mismo consumer, inmutabilidad de owner
  ante generación vieja y generación de providers al liberar con PUT vacío.
- make-check.log: gate local completo PASS para Placement y Glance.
- live/restart-proof.json: reservas persistidas tras SIGTERM exit 0 y nuevo
  ejecutable. Las generaciones/owner también se leen desde SQL en las suites.
  No se reinició MySQL en esta unidad; B11 verifica ese cierre transversal.

Las cabeceras Cache-Control y Last-Modified se conservan crudas. Se valida el
formato HTTP date y sólo se normaliza su valor temporal, además de request_id,
origen de Location y tokens Vary ya permitidos en B07. No se normalizan tipos,
status, códigos de error, owner o generaciones. created_at de allocations se
persiste mediante migración v5; el Last-Modified procede de las reservas, no de
un valor fijo. Los errores desde 1.23 incluyen code, en particular
placement.concurrent_update en conflictos de consumer.

source-files.sha256 identifica fuentes/inputs ensayados y provenance.json el
hash del ejecutable. Las migraciones v1–v4 están intactas. Las consultas al original
son por HTTP; el SQL de limpieza es sólo lectura. Los IDs project/user son
preexistentes en Placement para evitar registros globales imposibles de limpiar.
original-cleanup.json confirma ausencia de filas propias y metadatos globales
sin cambios. identity-cleanup.json y lab-final-state.json registran identidad
eliminada y VM apagada; procesos, DB y secretos locales fueron descartados.

## Reproducción y límites

Usar schemas vacíos propios y permisos separados como en B09-A. Ejecutar
make placement-test-allocations, placement-test-catalogs, placement-test-mysql,
placement-test-binary y placement-test-upgrade, cada uno en otro schema; make check.
Ejecutar scripts/placement_consumer_differential.py con --python-url, --go-url,
--project-id y --user-id preexistentes, los tres --*-token-file privados,
--output nuevo y --allow-mutations. Ejecutar los runners B09-A/B07/B08 aparte.
No sobrescribir S2 ni estos resultados.

El máximo negociable es 1.36 para allocations del perfil; durante este incremento
las otras rutas permanecen limitadas explícitamente a su perfil 1.8 mediante
406, en vez de tratar versiones nuevas como equivalentes. B10 amplía las rutas
que usa el resource tracker (providers/aggregates/inventories) y candidates;
no se promete toda Placement 1.36. No se cambian endpoints Nova ni Excel.
