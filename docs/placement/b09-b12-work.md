# Trabajo autorizado: terminar B09–B12

Inicio: 2026-10-29, base 8fa51ae (B09-A). El usuario pidió terminar hasta B12.
Se conserva el alcance parcial de Placement necesario para Nova; B13–B15 son
la sustitución reversible y el boot, que no se atribuyen a este cierre.

1. B09: formatos 1.12/1.28/1.34, lecturas owner/generation, CAS del consumer,
   liberación vacía, rollback, conflictos y comparación viva. Actualizar PR #8.
2. B10: candidates del inventario/uso/traits persistidos, filtros observados
   resources/limit/root_required, generación compatible 1.10/1.36 y dependencias
   de las rutas del resource tracker. No ignorar filtros no implementados.
3. B11: auditoría transversal de migración, aislamiento de cuentas/esquemas,
   invariantes y concurrencia, reinicio de proceso y MySQL, recuperación y cleanup.
4. B12: identidad Nova y roles/scope reales, permisos positivos/negativos por
   ruta, token ausente/inválido/revocado/expirado y fallos del validador sin bypass.

Cada unidad conserva contrato, make check, pruebas dirigidas y evidencia nueva
sin alterar S2/Excel. Los errores/generaciones no se normalizan para esconder
incompatibilidades. La VM se usa sólo para verificaciones necesarias y queda
apagada al terminar; no se reemplazan endpoints ni se ejecuta DevStack local.
