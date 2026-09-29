# Matriz final de trazabilidad Sprint 2

| Objetivo | Requerimiento | Backlog | Artefacto | Prueba | Evidencia | Estado |
|---|---|---|---|---|---|---|
| OE2 | RF01 | B01 | Auditoría/guía VM | Preflight host | `evidence/sprint2/environment/host-audit.txt` | Bloqueado |
| OE2 | RF01 | B02 | `infra/devstack/setup-devstack.sh`, `versions.lock` | `make devstack-verify` | `docs/sprint2/BLOCKERS.md`; no salida de servicios | Bloqueado |
| OE2 | RF01 | B03 | `smoke-test.sh`, `boot-instance-test.sh` | `make smoke` | `docs/sprint2/BLOCKERS.md`; sin VM | Bloqueado |
| OE1 | RF02 | B04 | `placement-architecture.md`, `placement-data-model.md` | Inspección upstream + revisión real pendiente | `evidence/sprint2/placement/upstream-source-audit.txt` | En progreso |
| OE1 | RF02 | B05 | `placement-api-scope.md` | API baseline pendiente | Código de microversiones upstream; sin respuestas | En progreso |
| OE1 | RF02 | B06 | `nova-placement-flow.md` | Boot + logs Nova pendiente | Auditoría del cliente Nova, sin tráfico | Bloqueado |
| OE1 | RF10 | B19 | `glance-architecture.md`, `glance-api-scope.md`, `glance-data-model.md` | Upload/download + Nova pendiente | `evidence/sprint2/glance/upstream-source-audit.txt` | En progreso |

OE1 no se declara cumplido hasta contrastar flujos y alcance con el laboratorio. OE2 está bloqueado hasta instalación, smoke tests y baseline reales. RF03–RF09 y RF10 siguen sin implementación Go.
