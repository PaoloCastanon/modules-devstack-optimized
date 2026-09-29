# Decisiones de diseño (provisionales hasta baseline)

## ADR-01: persistencia Placement

- **Contexto:** API exige provider/consumer generation, inventario y allocations transaccionales; el modelo upstream inspeccionado tiene relaciones lógicas sin todas las FK SQL.
- **Alternativas:** reutilizar esquema original; esquema propio compatible por API; esquema intermedio que importe/exporte datos.
- **Ventajas:** esquema original facilita sustitución sobre DB existente; propio reduce acoplamiento; intermedio permite migración gradual.
- **Desventajas:** original obliga a seguir migraciones Python; propio requiere migración/reconciliación; intermedio duplica complejidad.
- **Decisión:** esquema propio, compatibilidad por API, con DB separada en laboratorio.
- **Justificación:** el criterio RF09 se verifica por consumidores HTTP y evita escribir sobre DB original durante comparación diferencial.
- **Consecuencias:** se necesitan semillas y pruebas de equivalencia de estado; no se promete sustitución sin migración. Revisar si Nova requiere datos preexistentes.

## ADR-02: metadata y bytes Glance

- **Contexto:** código upstream separa `images`/`image_locations` del backend `glance_store`; RF10 exige ciclo mínimo de imagen.
- **Alternativas:** reutilizar schema Glance; metadata propia acotada; proxy a DB original. Para bytes: filesystem local o backend remoto.
- **Ventajas:** schema original puede facilitar migración; metadata propia mantiene alcance pequeño; filesystem único es observable y reproducible.
- **Desventajas:** schema original es amplio; metadata propia exige importación; filesystem limita escalado y puede diferir del backend del laboratorio.
- **Decisión:** metadata propia acotada y un store filesystem local **sujeto a confirmar** que el laboratorio usa o acepta ese backend.
- **Justificación:** el Excel restringe Glance al ciclo mínimo, sin multi-store. Escritura temporal y rename atómico permiten evitar `active` sin bytes completos.
- **Consecuencias:** registrar ruta configurada real, permisos, limpieza y hash; si DevStack usa otro store, documentar incompatibilidad antes de cambiar alcance.

## ADR-03: autenticación

- **Contexto:** clientes Nova/OpenStack usan Keystone y políticas; omitir auth invalidaría RF08/RNF07.
- **Alternativas:** confiar sólo en red privada; validar token vía middleware Keystone; replicar políticas completas.
- **Ventajas:** middleware conserva integración; políticas completas maximizan equivalencia.
- **Desventajas:** red privada no cumple requisito; middleware añade dependencia; políticas completas son más amplias que el laboratorio.
- **Decisión:** middleware de validación Keystone y políticas mínimas de rutas seleccionadas, verificadas con casos autorizados y rechazados.
- **Justificación:** conserva la barrera de autenticación sin reimplementar Keystone.
- **Consecuencias:** comparar 401/403 y roles con baseline; no almacenar tokens en fixtures.
