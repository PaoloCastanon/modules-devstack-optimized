# Nova → Glance

Según [arquitectura de Nova](https://docs.openstack.org/nova/latest/admin/architecture) y el código upstream SHA `dcae7ced...`, `nova.image.glance.API()` es usado por Compute; `nova/compute/manager.py` consulta metadata de imagen y el driver libvirt prepara el disco, descargando/copiando desde Glance según backend y caché. Conceptualmente, el usuario solicita `server create --image`; Nova valida imagen, Placement asigna host, Compute obtiene contenido y crea la VM. Neutron proporciona red. El endpoint Image, cabeceras, descargas concretas y cache deben contrastarse con logs reales; este documento no afirma una request observada.

Validación pendiente: crear una VM de prueba, capturar IDs y tiempos, logs Nova/Glance sanitizados, verificar `GET /v2/images/{id}` y `GET /v2/images/{id}/file` si ocurren, inspeccionar configuración de store/caché. `diagrams/nova-glance-sequence.mmd` es una secuencia de diseño pendiente de observación.
