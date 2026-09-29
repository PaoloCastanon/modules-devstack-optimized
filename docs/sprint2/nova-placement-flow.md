# Nova → Placement

El código de Nova stable/2026.2 SHA `dcae7ced...` muestra `nova/scheduler/manager.py` llamando `get_allocation_candidates`; `nova/scheduler/client/report.py` compone `GET /allocation_candidates?<resources.to_querystring()>` con `SAME_SUBTREE_VERSION='1.36'`. La respuesta incluye `allocation_requests` y `provider_summaries`. Scheduler filtra/pesa hosts y `claim_resources` solicita `PUT /allocations/{consumer_uuid}` usando la versión negociada; `nova-compute` publica inventario/traits mediante resource tracker. La petición lleva token de servicio Keystone, request ID y header de microversión según cliente HTTP. `CONSUMER_GENERATION_VERSION='1.28'` aparece en el cliente para operaciones de allocations. [Auditoría](../../evidence/sprint2/placement/upstream-source-audit.txt).

Cuándo ocurre: en scheduling del `server create` y en actualizaciones de recursos/consumo. Parámetros efectivos, token role, microversión negociada, status, logs y resultado de una VM **no fueron observados**; se capturarán con `nova-placement-flow.sh` y `boot-instance-test.sh`. La secuencia en `diagrams/nova-placement-sequence.mmd` distingue la inferencia del código de un trace real.

Fuente complementaria: [Placement usage](https://docs.openstack.org/placement/latest/user/) y [Placement API](https://docs.openstack.org/api-ref/placement/).
