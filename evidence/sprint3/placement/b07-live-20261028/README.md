# Comparación en vivo B07 — 2026-10-28

Se ejecutó providers/inventories 1.0 contra Placement Python
`f4a89d3803cb23df22b8714569889ab39c918c34` en `tt-openstack-lab` y el binario Go
con MySQL 8.0.46 independiente. **Ambos usaron el mismo Keystone real**, con
clientes admin, service y reader y una credencial del validador limitada a service.
No se cambiaron endpoints de Nova ni la configuración del Placement original.

| Resultado final | Casos |
|---|---:|
| Estado HTTP idéntico | 38/38 |
| Estado observable por API, antes y después, idéntico | 38/38 |
| Respuesta y estado idénticos bajo comparación estricta | 27/38 |
| Diferencias pendientes | 11/38 |

**El comparador terminó con código 1**, como corresponde a diferencias reales;
no es un pase de compatibilidad completa ni cierra B07. `make check` y las suites
de regresión/integración sí aprobaron sus criterios respectivos.

## Datos conservados

- [Reporte íntegro](run/report.json): requests canónicas sin token, respuestas
  con body textual, headers permitidos, snapshots por API, rutas de diferencias,
  resumen y verificación de limpieza. Los JSON individuales contienen cada caso.
- [Resumen de casos](case-summary.md): status y coincidencia de estado/respuesta.
- [Primer ensayo](initial-findings.json): 34 casos sobre `1e95402`, 32 status y
  9 comparaciones estrictas coincidentes. El ensayo final añade cuatro casos de
  UUID/collation; los denominadores difieren.
- [Procedencia](provenance.json): SHA upstream, columnas/collations inspeccionadas,
  hashes de migraciones/binario y estados reales de los historiales MySQL.
- [Upgrade](upgrade-proof.json): provider e inventory conservados al pasar una
  DB v1 real a v2, con generation intacta; canario eliminado después.
- [Limpieza Keystone](cleanup-proof.json): eliminación y ausencia comprobada de
  los recursos de identidad creados para esta ejecución.

Cada caso usa UUID/nombre nuevos e idénticos en ambos servicios. Antes de escribir,
el runner exige 404 para los UUID que podría crear; nunca borra un UUID existente
al hacer ese preflight. Las listas se filtran por esos recursos para no comparar
el provider real de Nova con una DB Go vacía. Finally elimina únicamente esos UUID
y confirma 404: **todas las eliminaciones quedaron verificadas**. Los snapshots
son consultas HTTP del estado de dominio; no representan dumps SQL completos.

La normalización se limita a valores de request_id de errores, el origen
configurado en Location y la capitalización de tokens de Vary, que HTTP trata
sin distinguir mayúsculas. JSON se compara por estructura y números exactos
usando Decimal, sin tolerancia; no se convierten booleanos, strings, null, arrays,
generation, status ni errores. UUIDs y nombres ya son iguales y no se reescriben.

## Correcciones verificadas

1. Enteros matemáticos como `3.0`/`3e0` se aceptan sin usar float64 para parsear
   generation; una fracción real se rechaza.
2. En query 1.0 se toma el último valor repetido, como el original.
3. Se reproducen igualdad por mayúsculas/espacios finales y unicidad de nombres.
   La migración v2 no altera el SQL/checksum de v1.
4. allocation_ratio se guarda como FLOAT y se lee mediante CAST a CHAR, porque
   el driver Python obtiene la representación textual de MySQL. La prueba detectó
   que leer FLOAT mediante protocolo binario aún devolvía otra precisión.
5. UUIDs de path no se convierten de compacto a formato con guiones; las mayúsculas
   sí se resuelven. Una query UUID compacta se rechaza como en el original.
6. El 401 usa la representación real del middleware Keystone y no anuncia headers
   de microversión. Se ajustaron errores estables de permisos, generation, clases
   desconocidas y provider inexistente en inventories; la comparación conserva
   los detalles que aún difieren.

## Diferencias pendientes

Diez casos difieren únicamente en `response.body.errors[0].detail`:
missing-name, null-name, unknown-field, rename-generation-field, reserved-total,
zero-total, null-total, mixed-invalid, uuid-compact-query y unsupported-media.
Sus status, tipos del contenedor de errores y efectos de estado coinciden.

El caso unacceptable coincide en 406, pero Python devuelve texto plano y Go
JSON. El reporte conserva tanto la diferencia de body como Content-Type.
No se eliminaron estos campos ni se declararon equivalentes para hacer pasar el gate.

## Límites y limpieza

Además de la matriz, se ejecutaron con race detector las suites de MySQL,
binario y colisión de upgrade, cada una con un esquema vacío nuevo. La colisión
de nombres en datos v1 dejó v2 dirty, mantuvo ambas filas y rechazó reintentos
automáticos. No se simuló pérdida de energía ni reinicio del servidor MySQL.

Se eliminaron el contenedor descartable, los providers de prueba, las dos
application credentials, su usuario/proyecto y los archivos privados temporales.
La VM se inició para esta prueba y volvió a `shut off`. Las capturas S2 y el Excel
se conservaron. No se probó boot Nova ni microversiones posteriores a 1.0.
