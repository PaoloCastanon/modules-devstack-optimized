# Matriz final de trazabilidad Sprint 2

| Objetivo | Requerimiento | Backlog | Artefacto | Prueba | Evidencia | Estado |
|---|---|---|---|---|---|---|
| OE2 | RF01 | B01 | Terraform/Ansible VM | Auditoría Ubuntu, recursos, sudo y virtualización | `evidence/sprint2/environment/vm-audit.txt` | Hecho |
| OE2 | RF01 | B02 | `infra/devstack/setup-devstack.sh`, `versions.lock` | `make lab-provision` y repetición sin reinstalar | `evidence/sprint2/devstack/service-list.json`, `endpoint-list.json` | Hecho |
| OE2 | RF01 | B03 | `smoke-test.sh`, `boot-instance-test.sh` | `make smoke` | `evidence/sprint2/tests/boot-states.txt`, `smoke-verify.txt` | Hecho |
| OE1 | RF02 | B04 | `placement-architecture.md`, `placement-data-model.md` | Código desplegado, API y tablas MySQL | `evidence/sprint2/placement/versions.response.json`, `environment/database-schema-audit.txt` | Hecho |
| OE1 | RF02 | B05 | `placement-api-scope.md` | `make baseline`; microversiones 1.0–1.39 y llamadas Nova medidas | `evidence/sprint2/placement/nova-requests.log`, `candidates.response.json` | Hecho |
| OE1 | RF02 | B06 | `nova-placement-flow.md` | Boot + logs de Scheduler y Placement | `evidence/sprint2/nova/scheduler-placement.log`, `placement/nova-requests.log`, `tests/boot-states.txt` | Hecho |
| OE1 | RF10 | B19 | `glance-architecture.md`, `glance-api-scope.md`, `glance-data-model.md` | Create/upload/PATCH/download/delete, auth 401, store y boot | `evidence/sprint2/glance/backend-audit.txt`, `image-download.sha256.txt`, `tests/boot-states.txt` | Hecho |

OE1 y OE2 están cumplidos para el alcance definido por el Excel. RF03–RF09 y la implementación Go de RF10 pertenecen a sprints posteriores. La retroalimentación formal del profesor queda pendiente de recibir y añadir al informe.
