# Glance Go como reemplazo permanente del original: verificación

Fecha 2026-11-20. VM local `tt-openstack-lab`. Rama `feat/glance-replace-original`,
binario de Glance Go sha256 `fa62bdd7…` (el de la campaña v4).

1. `install`: `{"installed": true, "listen": 19292, "notifications": "messagingv2", "copied": [{"name": "cirros-0.6.3-x86_64-disk", "result": "copied", "size": 21692416, "owner_kept": true}], "image_served_by": "glance-go", "glance-go.service": "active", "devstack@g-api.service": "inactive"}`.
2. `openstack image list` a través de `/image`: CirrOS `active`, servido por Glance Go.
3. `openstack server create --image <cirros> --flavor 42 --wait`: el servidor llegó a `ACTIVE` y luego se borró.
4. `use-original`: `{"image_served_by": "original", "backend_port": 60999, "devstack@g-api.service": "active"}`. `use-go` lo devolvió a Glance Go.
5. Después de reiniciar la VM, `status` dio `{"image_served_by": "glance-go", "glance-go.service": "active", "devstack@g-api.service": "inactive"}` y CirrOS seguía `active`.
