# Registro de evidencia Sprint 2

Los criterios se citan del Excel original. «Resultado» separa comprobación realizada de ejecución pendiente; un archivo de script no demuestra que DevStack funcione.

| Backlog ID | Requerimiento | Criterio de aceptación | Estado | Evidencia | Comando utilizado | Resultado | Commit | Observaciones |
|---|---|---|---|---|---|---|---|---|
| B01 | RF01 | Servidor o VM disponible y documentado | Bloqueado | `evidence/sprint2/environment/host-audit.txt` | `cat /etc/os-release; systemd-detect-virt; sudo -n true; nproc; free -h; df -h; curl` | Host CachyOS no VM, sudo no interactivo ausente | `e4fefb2` | Falta VM dedicada |
| B02 | RF01 | DevStack inicia y servicios críticos responden | Bloqueado | `infra/devstack/setup-devstack.sh`, `versions.lock`, `docs/sprint2/BLOCKERS.md` | `git ls-remote` para rama/commit; `bash -n`, `shellcheck` | Scripts preparados; `stack.sh` no ejecutado | `e4fefb2` | SHA de servicios instalados `UNINSTALLED` |
| B03 | RF01 | Suite smoke ejecuta casos críticos | Bloqueado | `infra/devstack/smoke-test.sh`, `scripts/baseline/boot-instance-test.sh` | `bash -n`, `shellcheck` | Sintaxis válida; smoke no ejecutado | `bdc822c` | Falta OpenStack |
| B04 | RF02 | Arquitectura/componentes/dependencias documentados | En progreso | `docs/sprint2/placement-architecture.md`, `evidence/sprint2/placement/upstream-source-audit.txt` | `rg` modelos/handlers Placement clone SHA `f4a89d3` | Upstream documentado; proceso real no analizado | `a0083f7` | No se cierra con documento solo |
| B05 | RF02 | Inventario rutas/microversiones dentro/fuera | En progreso | `docs/sprint2/placement-api-scope.md` | `rg version_handler`, `rg SAME_SUBTREE_VERSION` | Versiones de código identificadas; sin versión servidor probada | `a0083f7` | Falta captura baseline |
| B06 | RF02 | Llamadas/datos de flujos críticos | Bloqueado | `docs/sprint2/nova-placement-flow.md`, `evidence/sprint2/placement/upstream-source-audit.txt` | `rg get_allocation_candidates nova/scheduler` | Código inspeccionado; no tráfico observado | `a0083f7` | Falta boot VM/logs |
| B19 | RF10 | Especificación Glance endpoints/estados/store/auth/flujos | En progreso | `docs/sprint2/glance-api-scope.md`, `glance-data-model.md`, `evidence/sprint2/glance/upstream-source-audit.txt` | `rg '^class Image' glance/db/sqlalchemy/models.py` | Modelo upstream verificado; store real no inspeccionado | `a0083f7` | Falta Glance desplegado |

Las pruebas locales y el bloqueo seguro del instalador están en `evidence/sprint2/tests/local-checks.txt` y `setup-guard.txt`: `make docs-check` pasó; `make sprint2-verify` falló por ausencia de servicios, baseline y VM, como debe hacerlo. La evidencia de endpoint y VM sólo se creará al ejecutar en la VM.
