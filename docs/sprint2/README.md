# Sprint 2 / Etapa 2

Proyecto: reimplementación acotada de Placement y Glance en Go. Integrantes: Paolo Castañon Barrera y Juan Claudio Cárdenas Uribe. Profesor guía: Alejandro Mellado Gatica. Fecha límite: 2026-10-02.

La fuente de verdad es `Planificacion_Trabajo_Titulo_OpenStack_Placement_Glance.xlsx`, revisada en [excel-requirements-map.md](excel-requirements-map.md). El laboratorio dedicado Ubuntu 24.04 corre en una VM libvirt gestionada por Terraform y Ansible. Los servicios y el baseline real quedaron verificados el 2026-10-02; [sprint2-evidence.md](sprint2-evidence.md) relaciona cada criterio con su prueba y [BLOCKERS.md](BLOCKERS.md) registra límites pendientes.

## Sistema

OpenStackClient autentica contra Keystone y usa el catálogo de servicios. Nova API recibe la creación de VM; Nova Scheduler consulta candidatos y reclama allocations en Placement; Nova Compute publica inventario y ejecuta la instancia. Glance mantiene metadatos en base de datos y binarios en un image store; Nova obtiene la imagen para el arranque. Neutron aporta la red. El smoke test y los baselines contrastan esos componentes con respuestas reales; los diagramas conservan inferencias donde no se capturó el intercambio HTTP interno exacto.

Fuentes: [DevStack Quick Start](https://docs.openstack.org/devstack/latest/), [Placement API](https://docs.openstack.org/api-ref/placement/), [Glance Image API v2](https://docs.openstack.org/api-ref/image/v2/), [Nova architecture](https://docs.openstack.org/nova/latest/admin/architecture).
