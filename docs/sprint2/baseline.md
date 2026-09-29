# Línea base del sistema original

No hay línea base ejecutada: `evidence/sprint2/{placement,glance,nova,tests}` contiene solamente auditoría de código upstream y host. `capture-baseline.sh` realizará GET de versión, providers, inventories, traits, resource classes, allocation candidates, imágenes y metadata; cada petición guarda método/ruta, headers no secretos, status, headers de respuesta sanitizados y cuerpo. Las fixtures se copian con `tests/compatibility/*_baseline.sh`. Los tests mutantes, errores, upload/download y tráfico real de boot siguen pendientes y deben añadirse tras conocer configuración y recursos de la VM.

No se registran respuestas inventadas. Todos los comandos deben ejecutarse contra DevStack original antes de iniciar los servicios Go. El token se mantiene sólo en memoria de proceso y no se imprime.
