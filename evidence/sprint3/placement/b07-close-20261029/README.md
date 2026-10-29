# B07 — cierre de providers/inventories priorizados

Fecha: 2026-10-29. [Aceptación técnica y fronteras](../../../../docs/placement/b07-acceptance.md).

La campaña final usa Placement Python `f4a89d3803cb23df22b8714569889ab39c918c34`,
el binario Go de este incremento, MySQL 8.0.46 aislado y **Keystone desplegado**.
Los clientes admin/service/reader son credenciales de aplicación de roles
restringidos y expiración, en usuario/proyecto nuevos y temporales. El validador
Go utiliza una credencial service, sin admin. No se sustituyen servicios de Nova.

| Ensayo | Casos | Mismo status | Mismo estado | Estricto | Salida |
|---|---:|---:|---:|---:|---:|
| [1: matriz original corregida](attempt-1/run/report.json) | 38 | 38 | 38 | 38 | 0 |
| [2: ampliación antes de corregir prioridades](attempt-2/run/report.json) | 53 | 48 | 53 | 47 | 1 |
| [3: cierre ampliado](attempt-3/run/report.json) | **57** | **57** | **57** | **57** | **0** |

La limpieza de providers fue verificada en los tres ensayos. El ensayo 2 no se
oculta ni se trata como aprobado: detectó prioridades de existencia, generation,
capacidad/clase y media type/política. La campaña final añade también las dos
órdenes de inventarios inválidos y la semántica Accept de escrituras.

## Contenido y procedencia

- `attempt-*/run/`: requests sin tokens, responses originales completas, headers
  permitidos, snapshots antes/después y diferencias con rutas JSON. Los archivos
  individuales y report.json son resultados observados del runner.
- `attempt-*/differential.log`: resumen del comparador y verificación de limpieza.
- `attempt-*/{repo,binary,collision}-test.log`: suites con esquemas vacíos distintos
  para HTTP/MySQL, ejecutable y colisión de migración, con detector de carreras.
- [Reinicio final](attempt-3/restart-proof.json): provider/inventories y generation
  conservados tras SIGTERM y un proceso nuevo, mismo MySQL y Keystone real; canario
  eliminado después y 404 comprobado.
- [Procedencia final](attempt-3/provenance.json): SHA upstream, hash del binario,
  toolchain, resumen, salida y hashes de todas las fuentes productivas relevantes,
  go.mod/go.sum y esquemas embebidos. `base_sha` identifica el commit anterior al
  cambio no confirmado en ese momento; los hashes de fuentes vinculan el candidato
  probado con el código publicado, sin atribuir la corrección al commit anterior.
- [Control negativo](schema-before.log): las ocho regresiones pendientes fallan
  sobre exportación intacta de `4d1976c` con el nuevo test; salida 1.
- [Gate local](make-check.log), [API con race](api-check.log) y
  [orden repetido](ordering-check.log): PASS. El último ejecuta la regresión
  específica 30 veces para detectar regresión a orden aleatorio de mapas.
- [Limpieza Keystone](keystone-cleanup.json) y [VM](vm-cleanup.json): usuario y
  proyecto ausentes, credenciales eliminadas y VM shut off.
- [Planificación original](planning-reference.json): criterio B07 consultado en
  Backlog!H10; archivo sin modificar.

Los esquemas y el [oráculo de 61 casos](../../../../tests/compatibility/fixtures/placement-schema-errors.json)
se extrajeron de la instalación fijada. El
[capturador de sólo lectura](../../../../tests/compatibility/capture_placement_schema_oracle.py)
exige el SHA y versiones de jsonschema/WebOb inspeccionados. Los tests Go
comparan sus errores sin requerir Python/OpenStack en ejecución. El capturador
no forma parte de make check; la producción es Go y no depende de ese script.

## Reproducción

Ejecutar primero `make check`. En un MySQL descartable, preparar tres esquemas
nuevos para `make placement-test-mysql`, `make placement-test-binary` y
`make placement-test-upgrade`, mediante DSN privados y cuentas separadas según
la [guía B07](../../../../docs/placement/b07-development.md). La prueba de
colisión deja dirty su esquema deliberadamente; nunca reutilizarlo para servir.

Para la campaña, crear otro esquema nuevo, migrar con cuenta dedicada y arrancar
el ejecutable con cuenta runtime y Keystone real. Usar identidades de prueba
nuevas y credenciales cortas limitadas por rol. No utilizar cuentas/DB originales.

En esta ejecución un reverse proxy temporal en loopback monta `/placement` sobre
el listener Go. Sólo retira ese prefijo de la ruta entrante; no cambia cuerpos,
tokens ni respuestas. PUBLIC_BASE_URL señala el proxy. Esto conserva el mismo
path público del original sin tocar Apache, Placement ni configuración de Nova.

```sh
python3 scripts/placement_differential.py \
  --python-url http://192.168.122.10/placement \
  --go-url http://127.0.0.1:18779/placement \
  --admin-token-file /private/admin.token \
  --service-token-file /private/service.token \
  --reader-token-file /private/reader.token \
  --output /new/empty-output \
  --allow-mutations
```

18779 es un puerto de proxy de ejemplo; reemplazarlo por el configurado.
El directorio de salida debe ser nuevo. El runner exige ausencia de cada UUID
antes de mutar y sólo elimina esos UUID. Las listas se filtran por recursos de la
campaña, no por el provider real de Nova. Mantiene las mismas normalizaciones:
valores request_id, origen configurado de Location y capitalización de Vary.
No elimina detail, tipos, status, generation ni Content-Type para conseguir PASS.

El mismo esquema Go conserva un canario para reiniciar el proceso y comparar
snapshots; al terminar, eliminar canario, credenciales, usuario/proyecto, proxy,
procesos Go y contenedor MySQL. Verificar ausencia y apagar la VM.

## Límites

Se acepta B07 para las rutas priorizadas 1.0, no toda la API ni Nova con Go.
B08–B10, cierre transversal B11/B12 y sustitución B13–B15 continúan pendientes.
No se reinicia el servidor MySQL ni se simula corte de energía. No hay benchmark.
Las capturas S2, Excel y reportes académicos se conservan.

En schema-before.log se limpiaron sólo espacios de líneas vacías para cumplir
git diff --check; no se modificaron los cuerpos JSON ni las diferencias.
