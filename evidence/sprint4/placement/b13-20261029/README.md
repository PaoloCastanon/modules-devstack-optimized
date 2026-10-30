# B13 — publicación Nova, candidates y rollback

Cierre técnico del perfil plano, 2026-10-29. Ver docs/placement/b13-integration.md.

cycle-01 publica provider/inventarios/traits mediante nova-compute real. La query
falló al construir un objeto del helper Nova; restauración verificada. No PASS.
cycle-02 repite con provider nuevo en Go, publica mediante Nova, compara estado
lógico y obtiene candidates con cliente Nova real 1.36. PASS, rollback verificado.
El original uWSGI estuvo detenido durante ambos cambios; se preservó Keystone y
Glance originales y no se editó Nova. No hay boot atribuido a B13.

Se conservan respuestas HTTP crudas, generaciones (original3/Go2), diferencias
lógicas, query/candidates, identidad validada en logs Go, configuración original
por hash y comprobación después de restaurar. No se normalizaron generaciones.
La eliminación intermedia fue sólo del provider Go del ensayo, sin allocations y
con la ruta/servicio original activos. Original no recibió ese DELETE.

runtime-failed-* registra AppArmor/configuración de tmpdir, con cleanup antes de
reintentar. Runtime final: MySQL nativo propio, cuenta app DML y Go dentro de la VM;
puerto 13316 y UUID diferentes del original. initial-cli-isolation.txt NO demuestra
separación: defaults-extra-file permitió una opción de ~/.my.cnf. Se corrigió el
probe/readiness a defaults-file+SOCKET. own-mysql-isolation.txt e isolation-proof.json
son la comprobación válida. Sólo se ejecutó SELECT durante el probe ambiguo;
bootstrap PyMySQL y DSN Go siempre apuntaron explícitamente al MySQL propio.

red-*.log conserva TDD de guards/identidad y el fallo de logging sin identidad en
discovery antes de su corrección. api-tests.log, guards.log y make-check.log PASS.
Fuente/binario exactos identificados en source-files.sha256 y runtime-final-binary.json.
reproduction/host-cycle.py.txt es el orquestador exacto del lab con rutas/IDs de
esta sesión, sin credenciales; los helpers públicos están en scripts/integration.
No reutilizar roots/IDs históricos para ejecutar otra sesión.

Backups Apache, cnf, DSN y tokens no están en Git. Root del generador retirado.
Al cierre B13 el original está restaurado; recursos/identidad propios y VM siguen
en uso por B14, cuyo cierre registra cleanup y apagado. B15 conserva la suite
integral y dos ejecuciones. No son resultados B17/B18 ni modificación Excel/S2.
