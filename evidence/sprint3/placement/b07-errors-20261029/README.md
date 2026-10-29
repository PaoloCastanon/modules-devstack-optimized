# B07 — tres correcciones de errores, 2026-10-29

Incremento sobre `dfd7624` después del pull de main y la integración de Glance
B25. **No es una nueva campaña en vivo Python↔Go**: se reproducen las respuestas
Python archivadas de tres casos B07 y se comprueba persistencia con MySQL real.
La última campaña en vivo sigue siendo la del [28 de octubre](../b07-live-20261028/README.md),
con 27/38 coincidencias estrictas y 11 diferencias observadas en aquel código.

| Caso archivado | Corrección verificada |
|---|---|
| reserved-total | 400 con el detalle original y UUID/clase reales; inventarios y generation intactos |
| unsupported-media | 415 con el media type recibido; conserva JSON, título, status y headers |
| unacceptable | 406 text/plain UTF-8 con cuerpo original para Accept: text/plain |

La prueba compara cuerpo y headers estables; sólo omite el valor volátil de
request_id y compara Vary sin distinguir mayúsculas. Conserva el tipo del cuerpo,
Content-Type, status, title y detail. Los fixtures de Sprint 2 y las capturas de
la campaña anterior permanecen intactos. El runner no se modifica.

## Ejecuciones y reproducción

- [Antes](replay-before.log): sobre una copia exportada de `dfd7624`, con el
  nuevo test de contrato, `go test ./internal/placement/api -run
  '^TestCapturedNegotiationAndCapacityErrors$' -count=1` termina en **1**;
  fallan los tres casos. No se modifica la base para esta reproducción.
- [Después](replay-after.log): los tres casos y sus pruebas de borde pasan con
  `go test -race -count=1 ./internal/placement/api -run
  '^(TestCapturedNegotiationAndCapacityErrors|TestCapacityErrorsUseRequestedProviderAndClass|TestUnsupportedMediaReportsParsedType|TestAcceptFailureIsSeparateFromMicroversionFailure)$' -v`.
- [Gate](make-check.log): `make check` PASS, incluidos formato, vet, race y build
  de ambos dominios. Regresiones de Glance incluidas.
- [MySQL](mysql-check.log): `make placement-test-mysql` PASS, con MySQL 8.0.46
  en contenedor nuevo, datos tmpfs y puerto sólo loopback. Esquema vacío
  `placement_go_test_error_contract`, cuentas migrador/runtime distintas y
  grants limitados. Se añaden rechazos por capacidad para VCPU/MEMORY_MB y una
  lectura SQL posterior que exige igualdad de inventarios y generación.
  Keystone es un servidor HTTP de fixture con el adaptador productivo.
- [Primer intento MySQL](mysql-check-initial.log): falló el aprovisionamiento
  del contenedor antes de ejecutar tests. mysqladmin ping devolvía listo aunque
  la autenticación root aún no estaba inicializada. El segundo intento exigió
  SELECT 1 autenticado por TCP, con un contenedor/esquema nuevos. No se debilitó
  ninguna guardia del código ni de la suite.

Para repetir la integración, provisionar un MySQL descartable y un esquema nuevo
con cuentas separadas según [B07](../../../../docs/placement/b07-development.md);
entregar DSN mediante archivos privados 0600 y ejecutar:

```sh
PLACEMENT_GO_MYSQL_TEST=1 \
  PLACEMENT_GO_TEST_MIGRATE_DSN_FILE=/private/placement-migrator.dsn \
  PLACEMENT_GO_TEST_APP_DSN_FILE=/private/placement-runtime.dsn \
  make placement-test-mysql
```

La suite deja dirty su esquema de prueba deliberadamente al final; nunca usarlo
para una segunda campaña. Aquí se retiraron ambos contenedores, datos y archivos
privados al terminar. La VM tt-openstack-lab permaneció shut off.

## Alcance y pendientes

El modelo distingue capacidad inválida de campos mal formados; el servicio añade
el contexto sin ejecutar escrituras. Las variantes cubren UUID/clases diferentes,
reserved mayor que total, ratio cero y capacidad truncada a cero. La negociación
406 de microversión conserva su tratamiento separado. Para 415 se prueban también
Content-Type ausente, parámetros y application/xml.

Quedan ocho diferencias históricas de validación JSON Schema sin resolver:
missing-name, null-name, unknown-field, rename-generation-field, zero-total,
null-total, mixed-invalid y uuid-compact-query. No se acredita 30/38 en vivo ni se
cierra B07. La equivalencia de HTML, prioridades de errores/middleware, todas las
combinaciones Accept, microversiones posteriores y Nova sigue fuera de este pase.
No se reinició MySQL ni se probó pérdida de energía.

Contrato consultado: Placement fijado
[handlers/inventory.py](https://github.com/openstack/placement/blob/f4a89d3803cb23df22b8714569889ab39c918c34/placement/handlers/inventory.py)
y [util.py](https://github.com/openstack/placement/blob/f4a89d3803cb23df22b8714569889ab39c918c34/placement/util.py),
además de los cuerpos originales archivados. [Procedencia y hashes](provenance.json).
