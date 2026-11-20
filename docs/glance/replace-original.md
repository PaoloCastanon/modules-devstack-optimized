# Reemplazo permanente del Glance original por Glance Go

`scripts/integration/glance_replace.py` deja Glance Go como el servicio de
imágenes de la VM del laboratorio, de forma permanente. Sobrevive a los
reinicios y Nova, Horizon y el cliente `openstack` lo usan sin ningún cambio.
El Glance original no se borra: queda detenido y deshabilitado, y se puede
volver a él con un comando.

## Uso (dentro de la VM, con `openrc admin admin` cargado)

```sh
python3 scripts/integration/glance_replace.py install --binary bin/glance --allow-mutations
python3 scripts/integration/glance_replace.py status
python3 scripts/integration/glance_replace.py use-original --allow-mutations   # volver al oficial
python3 scripts/integration/glance_replace.py use-go --allow-mutations         # volver a Glance Go
```

Antes de correr la suite B26 o una campaña, ejecuta `use-original`, porque las
dos parten del oficial. Si no lo haces, fallan cerradas al comprobar el sitio
de Apache.

## Qué hace `install`

| Pieza | Permanente |
|---|---|
| Base de datos | `glance_go` en el mismo MySQL de DevStack, con cuentas `glance_go_migrate` (DDL) y `glance_go_app` (solo DML), como exige Glance Go |
| Credencial | Usuario `glance-go` en Keystone, con el rol `service` en el proyecto `service` y una credencial de aplicación sin vencimiento |
| Servicio | `glance-go.service` (systemd, habilitado), binario y store en `/opt/stack/data/glance-go`, escuchando en `127.0.0.1:19292` |
| Notificaciones | Las mismas del oficial (`messagingv2`), copiadas de su `glance-api.conf`. La URL del broker queda en un archivo `0600` |
| Imágenes | Cada imagen activa del oficial se copia a Glance Go con el mismo ID y los mismos metadatos, y se verifican el MD5 y el SHA-512 |
| Ruta | `/image` en Apache apunta a Glance Go, con el resto de la configuración de DevStack igual; `devstack@g-api` queda deshabilitado |

La base de datos es distinta a la del oficial porque Glance Go usa su propio
esquema (ADR de bases separadas). Que use la base del oficial queda como
trabajo pendiente.

## Verificación (2026-11-20)

[Evidencia](../../evidence/sprint5/glance-replace-20261120/README.md):
- se copió CirrOS (21 692 416 bytes) con el mismo ID y el mismo dueño;
- Nova creó un servidor desde CirrOS servido por Glance Go, quedó `ACTIVE` y se borró;
- `use-original` y `use-go` funcionan en ambos sentidos;
- después de reiniciar la VM, Glance Go arranca solo y sigue sirviendo `/image`.
