# B10 — candidates del perfil plano

Base: cierre B09 0e59cf3. Implementado y validado el 2026-10-29 para el laboratorio
con providers planos. El servicio sigue siendo un subconjunto Placement para Nova.
No se afirma soporte de árboles, providers compartidos, reshaper o toda API 1.36.

GET /allocation_candidates obtiene una instantánea repeatable-read de providers,
inventarios, uso, traits y agregados de MySQL propio. El servicio deriva solicitudes
y summaries: no hay respuestas constantes ni consulta al original para resolverlas.
Capacidad agotada, min/max/step, traits requeridos/prohibidos y root_required afectan
el resultado; se descuenta el uso persistido y se verifica capacidad al sumar grupos.
Limit se aplica a candidatos elegibles. Grupos numbered/alfanuméricos, group_policy,
in_tree y same_subtree se evalúan para árboles planos: dos grupos aislados no pueden
usar el mismo root, y un grupo sin recursos requiere estar en same_subtree desde 1.36.
Los parámetros mal formados o desconocidos fallan explícitamente.

Representaciones: 1.10 usa lista en allocations, 1.12 mapa por provider; 1.17 añade
traits; 1.27 incluye todos los inventarios; 1.29 parent null/root UUID; 1.34 mappings.
Cache-Control/Last-Modified siguen 1.15. Las fechas HTTP se validan y normalizan
como metadato temporal; cuerpos, códigos, generaciones, capacidad y tipos no se
normalizan. Sólo se ordenan conjuntos sin deduplicar.

## Perfiles del resource tracker

La revisión Nova 247ea5d confirma estas versiones. Se admiten sólo sus rangos
seleccionados; una versión más alta en otra ruta recibe 406 explícito.

| Ruta | Máximo del perfil | Comportamiento incorporado |
|---|---|---|
| providers y provider individual | 1.20 | links allocations 1.11; root/parent 1.14; in_tree; required 1.18; POST 200 con body desde 1.20; caché/Last-Modified 1.15 |
| inventories e inventory GET | 1.26 | reserva igual al total y capacidad cero desde 1.26; tiempos persistidos y error codes 1.23 |
| aggregates por provider | 1.19 | GET/PUT incluyen generation; PUT con CAS aumenta incluso si el conjunto no cambia |
| allocations consumer/provider | 1.36 | B09: formato moderno, consumer generation, mappings y tiempos de reservas |
| allocation_candidates | 1.36 | filtros/representaciones seleccionados de esta guía |
| catálogos/traits y provider usages | 1.8 | se conservan los contratos previos usados en Nova 1.6/1.7/default |

Un parent_provider_uuid no nulo se rechaza como fuera del perfil plano sin escribir;
no se acepta para luego ignorarlo. Un estado con MISC_SHARES_VIA_AGGREGATE también
falla explícitamente en candidates. Los agregados del perfil plano filtran pertenencia,
sin implementar recursos compartidos entre árboles. POST batch allocations y GET
usages globales no forman parte de las rutas priorizadas; B13–B15 verifica el flujo
completo antes de cualquier sustitución. No se promete boot por este cierre.

La migración v6 añade tiempos a provider/inventory, conservando v1–v5. Los tiempos
históricos desconocidos se inicializan al migrar; no se inventa su antigüedad. Los
cambios del inventario actualizan su propia marca, y las reservas su created_at.
Los timestamps también se preservan en reinicio/backup de B11.

## Validación

[Evidencia B10](../../evidence/sprint3/placement/b10-close-20261029/README.md):
**44/44** casos vivos estrictos y ocho respuestas de preparación sin diferencias,
con MySQL/Keystone reales. Incluye filtro Nova con root_required, uso real tras claim,
limit, granular requests, same_subtree, errores, reader/token y versiones del tracker.
Regresiones B09 moderno **24/24**, B09-A **60/60**, B07 **57/57**, B08 **82/82**.

La suite SQL prueba filtros de aggregate con asociación positiva, CAS de aggregate,
y lecturas de candidates durante 40 reemplazos concurrentes: generation/usage mantienen
la misma instantánea. Las otras cinco suites MySQL y make check pasan. La comparación
viva no crea agregados globales imposibles de limpiar; sólo PUT [] en el original.
Una clase CUSTOM fresca acota todas las consultas para excluir el provider Nova real.
Las solicitudes, respuestas y estados crudos se guardan. Cleanup HTTP y lectura SQL
posterior confirman ausencia de filas propias; no hay SQL de escritura al original.
La identidad/VM se comparten temporalmente con B11/B12 y se limpiarán al cierre final.

Reproducir con make placement-test-candidates en schema nuevo vacío y opt-in;
el runner scripts/placement_candidates_differential.py exige --allow-mutations,
carpeta nueva, URLs, tokens privados y project/user ya existentes en Placement.
No sobrescribir S2/Excel ni cambiar endpoints Nova. Ver comandos en evidencia.
