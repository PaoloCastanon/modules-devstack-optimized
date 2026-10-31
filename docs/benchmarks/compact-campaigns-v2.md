# Campañas compactas, revisión 2

2026-10-31. El usuario pidió recuperar espacio y campañas de menor duración.
Los perfiles anteriores se conservan como historia; esta revisión define el
alcance preliminar de la siguiente ejecución, sin mezclar intentos ni datos.
Estado: perfiles y adaptación Placement implementados; checks locales aprobados.
Piloto real, medición y aceptación de revisión 2 pendientes. Glance y conjunto
conservan sus tareas de instrumentación pendientes.

| Campaña | Diseño compacto | Carga y calentamiento, sin preparación |
|---|---|---|
| Placement | Seis cargas, concurrencias 1/16, tres pares Python/Go; 72 ventanas | 48 minutos |
| Glance | Nueve cargas y tres tamaños, concurrencias 1/16, tres pares; 108 ventanas | 72 minutos |
| Conjunta | Cuatro variantes, lotes 1/2, tres repeticiones, imagen fría; 24 bloques/36 boots | Depende del boot, no de una ventana HTTP |

Ventanas HTTP: 10 s de calentamiento y 30 s medidos. Con preparación, controles
y limpieza, presupuestar aproximadamente 1–2 h para Placement, 2–3 h para Glance
y 30–90 min para la conjunta; son estimaciones a revisar con el piloto, no plazos
garantizados. La conjunta mantiene originales/originales, Go/original,
original/Go y Go/Go. Caché caliente y lotes de cuatro quedan fuera de esta revisión.

Se preservan autenticación real, comparación de estado/bytes, limpieza, boot
ACTIVE, allocations y muestras CPU/PSS; un error no se convierte en éxito.
Tres repeticiones ofrecen menos información de variación que cinco: publicar
resultados por repetición, mediana y rango. No afirmar significación estadística,
intervalos precisos o p99 de boot fiables. Ampliar después sólo las celdas cuya
variación o cobertura requiera una campaña adicional, con revisión previa.

## Instrumentos y aceptación antes de medir

- Perfiles: placement-protocol-compact-v2.json, glance-protocol-compact-v2.json
  y joint-protocol-compact-v2.json, en esta carpeta.
- Placement deriva matriz, supervisor, análisis y publicación del protocolo.
  Las constantes de la revisión larga fueron eliminadas y las guardas de
  publicación probadas con revisión 2. Publicar 72/72 bajo v2 no acredita 240/240 bajo v1.
- Ajustar análisis/publicación para distinguir el alcance preliminar y no
  atribuir precisión a un bootstrap de sólo tres repeticiones. Congelar hashes,
  fuentes y orden tras un piloto. No recortar una campaña a mitad de ejecución.
- Glance y conjunto necesitan sus runners, preflight y pilotos; conservar los
  criterios de propiedad, integridad, integración y restauración del runbook.
- Guardas de espacio: mínimo 20 GiB libres en el filesystem anfitrión del qcow2
  antes de comenzar; supervisión durante la campaña, parada y restauración
  anticipadas antes de que el espacio se agote. Guardar estado en rutas nuevas.
- Limitar logs operacionales en la VM, con configuración registrada; rotar fuera
  de las ventanas medidas y conservar diagnósticos recientes. Recolectar datos
  de las pruebas en streaming/comprimidos; no omitir errores ni muestras para
  aparentar menor volumen. Mantener las evidencias previas.
- Actualizaciones automáticas de la VM suspendidas temporalmente durante la
  campaña. Una sola campaña activa. Restaurar originales, limpiar recursos
  propios y apagar al terminar. No hay merge automático.

## Recuperación de espacio observada

Se limpiaron cachés HTTP/código de Chromium, metadatos de registro pnpm y HTTP
pip: 1,56 GiB netos recuperados inicialmente. Dentro de Ubuntu se encontraron
30.661.216.603 bytes en syslog, 2.206.300.560 en horizon_access.log y cerca de
4 GiB en el journal. Se conservaron colas comprimidas recientes (128/32 MiB de
entrada, respectivamente), extractos de journal de los dos incidentes y toda
la evidencia de campañas; se redujeron logs antiguos y el journal a 256 MiB,
y se limpió la caché apt. Ningún disco de la VM fue eliminado.

Se activó discard=unmap en vda para ejecutar fstrim del espacio libre de Ubuntu.
Capacidad virtual conservada en 55 GiB; ocupación del qcow2 de 44,79 a unos
6,76 GiB y filesystem anfitrión con unos 39 GiB libres al terminar. Respaldo de
XML anterior y registro en reports/campaigns/vm-reclaim-20261031 del workspace
superior. Las colas de logs privadas permanecen dentro de la VM, en
/home/stack/tt-log-reclaim-20261031; no se incorporan a Git.

La configuración Terraform de vda incorpora discard=unmap; esquema libvirt
0.9.9 y terraform validate aprobados, sin ejecutar apply. El control de límites/retención debe prepararse
antes de medir para evitar reconstruir los mismos logs enormes.

Fuentes técnicas para la reclamación: [libvirt, atributos del driver de disco](https://libvirt.org/formatdomain.html)
y [QEMU, imágenes de disco](https://www.qemu.org/docs/master/system/images).
