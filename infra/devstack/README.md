# Laboratorio DevStack

Se instala únicamente en VM/servidor Ubuntu 24.04 dedicado. DevStack cambia paquetes, redes y servicios del sistema. `setup-devstack.sh` bloquea otros SO, ejecución como root, ausencia de sudo no interactivo y falta de recursos. `DEDICATED_LAB=YES` es una declaración explícita del operador de que comprobó la dedicación.

Copiar `.env.example` a `.env`, establecer secretos alfanuméricos únicos y ejecutar desde raíz `make devstack-setup`. `.env`, `local.conf` generado y logs están fuera de Git. No copiar logs crudos al repositorio porque podrían contener credenciales. `verify-devstack.sh` usa OpenStackClient; si no hay variables `OS_*`, carga el `openrc` de DevStack. `unstack-devstack.sh` detiene servicios; `clean-devstack.sh` exige otra vez `DEDICATED_LAB=YES` y sólo invoca el limpiador de DevStack.

`versions.lock` fija el commit de DevStack y registra versiones reales del resto después de instalación. Para reproducibilidad estricta de todos los proyectos, conservar los SHAs registrados y restaurar esos checkouts en la siguiente VM antes de la comparación experimental; el script actual fija sólo DevStack y rama de los servicios, no promete resolver idénticas dependencias en fechas futuras.
