# Glance Go en DevStack (plugin `glance-go`)

Glance Go se instala como un servicio más de DevStack, con su mecanismo estándar
de plugins. No necesita scripts del laboratorio ni instancias temporales: sirve
en cualquier instalación de DevStack con la base de datos MySQL y el store
filesystem de Glance (los valores por defecto).

## Uso

En `local.conf`:

```ini
[[local|localrc]]
enable_plugin glance-go https://github.com/PaoloCastanon/modules-devstack-optimized main
```

Después, `./stack.sh` como siempre. El servicio queda como `devstack@g-go`:

```sh
sudo systemctl status devstack@g-go
sudo journalctl -u devstack@g-go -f
openstack image list          # /image ya lo atiende Glance Go
```

`./unstack.sh` lo detiene y `./clean.sh` borra su raíz privada.

## Qué hace en cada fase de `stack.sh`

| Fase | Acción |
| --- | --- |
| `stack install` | Descarga Go (versión fija, sha256 verificado con `get_extra_file`) y compila `bin/glance` estático desde el checkout del plugin. |
| (DevStack) | Instala, configura y migra el Glance original (`g-api`), crea su usuario de servicio y el endpoint `image`, y sube CirrOS. |
| `stack extra` | Recrea `glance_go_shared` (su contabilidad de transferencias, como DevStack recrea `glance`), crea su raíz privada, inicializa el store, inicia `devstack@g-go` con `run_process`, apunta `/image` de Apache a Glance Go con `write_local_proxy_http_config`, y detiene y deshabilita `devstack@g-api`. Al arrancar, Glance Go adopta CirrOS y el resto de imágenes del original. |
| `unstack` | `stop_process g-go`. |
| `clean` | Borra `$DATA_DIR/g-go`. |

Glance Go lee el mismo `/etc/glance/glance-api.conf` que el original: la cuenta
y la base `glance` (`[database]`), el directorio de imágenes
(`[glance_store] filesystem_store_datadir`), el usuario de servicio de Keystone
(`[keystone_authtoken]`) y las notificaciones. El endpoint de Keystone no cambia.
El original sigue instalado (por ejemplo, para `glance-manage db purge`), pero no
atiende peticiones.

## Variables

| Variable | Por defecto |
| --- | --- |
| `GLANCE_GO_LISTEN` | `127.0.0.1:19292` |
| `GLANCE_GO_STORE_ROOT` | `$DATA_DIR/g-go/store-root` (lock y marca de alcance; los archivos van al directorio del original) |
| `GLANCE_GO_EXTRA_ENV` | vacío; otras `GLANCE_GO_*` separadas por espacios, por ejemplo `GLANCE_GO_MAX_CONCURRENT_TRANSFERS=16` |
| `GLANCE_GO_TOOLCHAIN_VERSION`, `GLANCE_GO_TOOLCHAIN_SHA256` | Go `1.27.4` y su sha256 de linux-amd64 publicado en go.dev |

Requisitos: `g-api` habilitado (viene por defecto) y acceso a go.dev y al proxy
de módulos de Go durante `stack.sh`.

## Verificación

`scripts/integration/glance_devstack_suite.py` reúne lo que antes hacían B26 y el
E2E del catálogo compartido, todo a través de `/image` hacia `devstack@g-go`:

```sh
source /opt/stack/devstack/openrc admin admin
python3 scripts/integration/glance_devstack_suite.py --flavor-id 42 \
    --network-id <red privada> --output <directorio nuevo> --allow-mutations
```

Casos: integración en DevStack (unidades, Apache y endpoint), autenticación que
falla cerrada, 404, ciclo de catálogo y bytes (con el archivo `<id>` en el
store), notificaciones contra una línea base del original
(`scripts/integration/fixtures/glance-notifications-original.json`), el CirrOS que
subió DevStack servido por Glance Go, Nova arrancando desde ese CirrOS y desde
una imagen subida por Glance Go, el flujo Nova → Glance y la limpieza.
