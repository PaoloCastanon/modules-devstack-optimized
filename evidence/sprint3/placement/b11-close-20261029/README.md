# B11 — durabilidad, aislamiento y restore

Base B10 b42c273, 2026-10-29. attempt-2/report.json registra **9/9** verificaciones
obligatorias PASS; exit 0, container_removed, volume_removed y private_files_removed
true. MySQL 8.0.46 en volumen propio, cuentas DDL/DML separadas y Keystone real.

Se sembraron provider, inventories, traits, aggregates y allocations por HTTP.
Después se verificó caída Go SIGKILL, MySQL SIGKILL con owner no confirmado dentro
de una transacción abierta y restore de backup en otro schema/cuenta. Las
instantáneas de ocho rutas HTTP coinciden; el dump de todas las filas se conserva
byte a byte y sus hashes se guardan. La cuenta runtime no puede DDL ni leer otro
schema; serve rechaza una cuenta con DDL y el sentinel queda intacto.

backup.sql contiene sólo el esquema propio con datos sintéticos, sin cuentas ni
credenciales MySQL/Keystone. El restore usa FOREIGN_KEY_CHECKS=0 sólo en su sesión
administrativa de replay; los datos restaurados se cotejan íntegros y el runtime
mantiene FK. El volumen, contenedor y secretos se eliminan en finally.

attempt-1 conserva un fallo del helper tras restart con binding aleatorio; no
se cuenta como pase. Se cambió a puerto explícito y Docker create/start con
ownership registrado. attempt-2 es el cierre válido; no se debilitó ninguna
aserción ni se añadieron bypass al ejecutable.

source-files.sha256 identifica las fuentes Go e inputs de los ensayos; cada
report incluye hash binario/backup/datos y respuestas crudas. Last-Modified se
valida como HTTP date y se normaliza su valor como en B09; SQL, generaciones,
owner, capacidad y tipos no se normalizan.

Reproducción:

```sh
python3 scripts/placement_persistence_campaign.py \
  --output "$NEW_B11_OUTPUT" --keystone-url "$REAL_KEYSTONE_V3_URL" \
  --validator-file "$PRIVATE_SERVICE_CREDENTIAL_FILE" \
  --admin-token-file "$PRIVATE_ADMIN_TOKEN_FILE" --allow-mutations
```

Requiere Docker y Go locales. El script crea DB/volumen propios y no toca la DB
original ni inicia DevStack. Los archivos privados deben quedar fuera del repo.
Ver contrato/límites en docs/placement/b11-acceptance.md. La limpieza de identidad
y apagado de VM compartida para B12 se registra en ese cierre final.
