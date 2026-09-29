# Sprint 2 / Etapa 2

Proyecto: reimplementación acotada de Placement y Glance en Go. Integrantes: Paolo Castañon Barrera y Juan Claudio Cárdenas Uribe. Profesor guía: Alejandro Mellado Gatica. Fecha límite: 2026-10-02.

La fuente de verdad es `Planificacion_Trabajo_Titulo_OpenStack_Placement_Glance.xlsx`, revisada en [excel-requirements-map.md](excel-requirements-map.md). Los documentos distinguen evidencia de código/documentación upstream de pruebas de un DevStack instalado. Este equipo no es un laboratorio dedicado y aún no se han ejecutado servicios ni baseline; [BLOCKERS.md](BLOCKERS.md) registra los pasos faltantes.

## Sistema

OpenStackClient autentica contra Keystone y usa el catálogo de servicios. Nova API recibe la creación de VM; Nova Scheduler consulta candidatos y reclama allocations en Placement; Nova Compute publica inventario y ejecuta la instancia. Glance mantiene metadatos en base de datos y binarios en un image store; Nova obtiene la imagen para el arranque. Neutron aporta la red. Las secuencias de este repositorio son hipótesis sustentadas por código upstream, pendientes de contrastar con trazas del laboratorio.

Fuentes: [DevStack Quick Start](https://docs.openstack.org/devstack/latest/), [Placement API](https://docs.openstack.org/api-ref/placement/), [Glance Image API v2](https://docs.openstack.org/api-ref/image/v2/), [Nova architecture](https://docs.openstack.org/nova/latest/admin/architecture).
