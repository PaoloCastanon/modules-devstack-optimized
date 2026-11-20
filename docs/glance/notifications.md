# Notificaciones de imagen en Glance Go

Glance Go envía las mismas notificaciones que el Glance oficial (`a3e5f490`,
`glance/notifier.py`, con el driver `messagingv2` de oslo.messaging). Con ellas,
los servicios que escuchan RabbitMQ (cobro, métricas, auditoría) se enteran de
los cambios de imagen igual que antes del reemplazo.

## Qué se envía

| Operación | Eventos, en orden | Contenido |
|---|---|---|
| `POST /v2/images` | `image.create` | imagen (`format_image_notification`) |
| `PATCH /v2/images/{id}` | `image.update` | imagen modificada |
| `PUT /v2/images/{id}/file` | `image.update` (saving), `image.prepare`, `image.upload`, `image.activate`, `image.update` (active) | imagen; `prepare` y `upload` agregan `os_glance_importing_to_stores` y `os_glance_failed_import` |
| Falla de la carga después de `prepare` | `image.upload` con prioridad `ERROR` | un texto, como el oficial |
| `GET /v2/images/{id}/file` | `image.send`, al terminar de enviar el cuerpo | `bytes_sent`, `image_id`, `owner_id`, `receiver_tenant_id`, `receiver_user_id`; prioridad `ERROR` si salió corto; nada si el cliente se fue antes |
| `DELETE /v2/images/{id}` | `image.delete` | imagen con `status=deleted`, `deleted=true` y `deleted_at` |

**Formato** (igual al oficial):
- envoltura `{"oslo.version": "2.0", "oslo.message": "<json>"}`;
- mensaje con `message_id`, `publisher_id` (`image.localhost`), `event_type`, `priority`, `payload`, `timestamp` (`2026-11-19 18:07:47.047579`, UTC) y `_unique_id`;
- contexto vacío, como en Glance (`self._notifier.info({}, ...)`): **los mensajes no llevan tokens**;
- exchange topic `glance` (el `control_exchange` de Glance) y cola `notifications.info` o `notifications.error`, enlazada con esa misma clave. Las dos son no durables y no se autoeliminan, como en oslo;
- mensajes persistentes, `application/json` y `utf-8`.

## Diferencia deliberada: no bloquea la API

El oficial envía la notificación dentro de la solicitud. Glance Go la deja en
una cola acotada (1024 mensajes) y un worker la publica en orden. Un broker
lento o caído nunca demora ni rompe una solicitud:
- mientras el broker no responde, se reintenta el mensaje más antiguo;
- si la cola se llena, los nuevos se descartan y se registra un aviso por corte;
- al apagar se intenta entregar lo pendiente durante 5 s.

## Configuración

Por defecto el driver es `noop`: no se envía nada.

| Variable | Equivale a | Valor |
|---|---|---|
| `GLANCE_GO_NOTIFICATION_DRIVER` | `[oslo_messaging_notifications] driver` | `noop` (por defecto) o `messagingv2` |
| `GLANCE_GO_TRANSPORT_URL_FILE` | `transport_url` | archivo `0600` con `rabbit://usuario:clave@host:5672/` (un solo host). Nunca se registra en los logs |
| `GLANCE_GO_NOTIFICATION_TOPICS` | `topics` | `notifications` |
| `GLANCE_GO_DEFAULT_PUBLISHER_ID` | `default_publisher_id` | `image.localhost` |
| `GLANCE_GO_DISABLED_NOTIFICATIONS` | `disabled_notifications` | tipos (`image.send`) o grupos (`image`), separados por comas |

En el laboratorio, `placement_runtime.py --notifications original` copia esos
valores desde el `glance-api.conf` del oficial. La suite B26 lo usa así; la
campaña sigue en `noop`, como la v3.

## Verificación

- **Pruebas Go** (`internal/glance/notify`): el contenido es igual campo por campo a 9 mensajes reales del oficial (`testdata/official-cycle.json`). Además, la envoltura, el orden, los reintentos, la cola acotada, el apagado, el filtro `disabled_notifications` y que no se filtren credenciales.
- **Pruebas del servicio**: las secuencias de crear, PATCH, carga (exitosa y fallida), descarga y borrado.
- **Con RabbitMQ real** (suite B26, casos `notifications_original` y `notifications_go`): el mismo ciclo por `/image`, primero con el oficial y luego con Go, compara secuencia, prioridad, clave de ruteo, emisor, propiedades AMQP, claves y valores. Dio **0 diferencias**. [Evidencia](../../evidence/sprint5/glance-notifications-20261119/README.md).
