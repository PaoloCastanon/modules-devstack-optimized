## PR #1 — revisión independiente

**HEAD revisado:** `bbe1d2bdc1aff51a8ee0b6534d5830bee47df00a`  
**Recomendación: approve-with-nits**, condicionada a los gates que estás ejecutando. No encontré un defecto bloqueante demostrado.

### Corrección confirmada

En el DevStack fijado `0fb9685c…`, `lib/nova` pasa `$NOVA_BRANCH` a `git_clone`. Para un checkout nuevo, `functions-common` ejecuta `git fetch origin "$git_ref"` y `git checkout FETCH_HEAD`: **acepta el SHA completo**, aunque la variable se llame `NOVA_BRANCH`.

Con `RECLONE=no`, un checkout existente no se actualiza. El preflight del PR y las comprobaciones posteriores evitan registrar una revisión diferente como válida. No observé nuevas exposiciones de credenciales ni ejecución insegura por las sustituciones: el SHA está restringido a 40 caracteres hexadecimales.

### Observaciones no bloqueantes

1. **La prueba añadida no forma parte de `make check`.**  
   Archivo del diff: `tests/compatibility/nova_pin.sh`, líneas **7–18**. El gate actual sólo hace `bash -n`/ShellCheck sobre los `.sh`; las aserciones de este script no se ejecutan. Además, el script verifica su propia sustitución con `sed`, sin comprobar que el instalador siga incluyendo `NOVA_SHA` entre sus variables. Conviene incorporarlo al gate y cubrir renderizado real o, como mínimo, la inclusión de `NOVA_SHA` en el instalador.

2. **Falta documentar la migración del `local.conf` anterior.**  
   Archivo del diff: `infra/devstack/local.conf.template`, línea **12**; relacionado con `setup-devstack.sh`, líneas **62–66**. Un laboratorio anterior conserva `NOVA_BRANCH=stable/2026.2`; el nuevo render contiene el SHA y el `cmp` rechaza reejecutar el instalador, incluso si Nova ya está en la revisión correcta. Es una consecuencia comprobable del guard de configuración existente, no corrupción ni actualización silenciosa. La reprovisión habitual de Ansible con marcador completo omite ese instalador, por lo que no se rompe ese flujo. Añadir instrucciones de migración segura para reinstalación/reintento manual resolvería la ambigüedad.

### Límites

No ejecuté instalación DevStack, pruebas contra Nova ni la nueva prueba en una VM. No publiqué reviews ni modifiqué archivos, ramas o VM. La instalación completa sigue pendiente, tal como indica la descripción del PR.

Fuentes primarias revisadas: [`functions-common`](https://github.com/openstack/devstack/blob/0fb9685c1bf22cd0b4d576d7a74f64195ad934fe/functions-common), [`lib/nova`](https://github.com/openstack/devstack/blob/0fb9685c1bf22cd0b4d576d7a74f64195ad934fe/lib/nova).
