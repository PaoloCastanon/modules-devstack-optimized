# Piloto joint-compact-v4 (boots Nova, Placement × Glance)

2026-11-28, 02:12–02:21 UTC. Código `9c3f830` (más el cambio del análisis que
usa POST→build en los cocientes), binarios `glance` `652a7337…` y `placement`
`91866d8f…`. 1 repetición, concurrencia 1/2/4: **12/12 bloques y 28 boots
válidos, limpieza completa**. No es resultado oficial. Tablas en
[analysis.md](analysis.md).

Intento 1 (no usado): bloque 1 inválido solo por `phases_ok`. Los eventos de
acción de Nova tienen resolución de 1 s y journald guarda como bytes las líneas
coloreadas de nova-compute. Se corrigió en `9c3f830`: las fases salen del
journal en µs.
