# Evidencia del primer incremento B07 — 2026-10-28

Las salidas adjuntas corresponden al código del incremento B07 y a sus gates
locales. [Contrato, comandos y límites](../../../../docs/placement/b07-development.md).

- `make-check.log`: gates locales completos, incluidos Glance y Placement Go.
- `api-fixtures.log`: regresión del flujo S2 y 404, sin conectar al original.
- `mysql-integration.log`: MySQL 8.0.46 real, transacciones y HTTP con Keystone fixture.
- `binary-integration.log`: binario real, migrate/serve, SIGTERM, reinicio y persistencia.

MySQL corrió en `tt-placement-b07-mysql`, contenedor descartable con puerto solo
loopback y datos tmpfs, imagen fijada por digest en la guía. Cada ejecución usó
un esquema vacío nuevo y dos cuentas limitadas a ese esquema. Ningún DSN,
password, token real ni configuración privada está incluido. La instancia y
sus datos se eliminaron al finalizar; para reproducir se necesitan esquemas nuevos.

El primer ensayo detectó MySQL error 1216 (FK), además del 1452 esperado; se
amplió la traducción y se verificó el rollback en una DB nueva. Una prueba del
binario reutilizaba un contexto HTTP que podía expirar durante su shutdown;
se corrigió la prueba para usar un contexto nuevo en la solicitud tras reinicio.
Los gates no se debilitaron y las salidas adjuntas son las ejecuciones finales.

Estos resultados no prueban Keystone real, comparación en vivo Python↔Go,
persistencia ante reinicio de MySQL/corte de energía ni integración Nova.
Las capturas originales S2 permanecen intactas. B07 sigue en progreso.
