# B14 — boot Nova con Placement Go

2026-10-29 (America/Santiago). **B14 completado técnicamente para el laboratorio.**
Base B13 55b0019, binario Go identificado en su evidencia (83433d9b…).
[Prueba real](../../evidence/sprint4/placement/b14-20261029/README.md).

La instancia 1af1c553-3927-431a-be9b-d7a1f3010006 pasó BUILD→ACTIVE el
2026-10-30T00:39:02Z (21:39:02 del 29 de octubre en Santiago). Usó la imagen
CirrOS original 525c5703-e0a5-4cee-9669-9e57fd89ca32, flavor m1.tiny (ID1)
y red shared 44193c7a-3e3b-4ae2-900e-e1e88856d4e3, seleccionados por ID.
No se tomó el primer resultado de un listado ni se modificaron imagen/flavor/red.

Placement Python uWSGI estuvo detenido y Apache /placement apuntó sólo a Go.
Nova original realizó candidates, consulta y claim; los logs Go registran la
identidad Nova validada por Keystone, PUT allocations 204 y DELETE allocations 204.
El consumer es el UUID real de la instancia y el provider es el compute
3b7cc290-1329-47d2-9502-890059c82d1b. Durante ACTIVE se verificó:

- VCPU=1, MEMORY_MB=512, DISK_GB=1, exactamente el flavor del ensayo.
- project_id=76fe49699db545d39b31ddd2ad0abaff y user_id=2ff6c5c299214623a95877cfc424aa39,
  iguales a los propietarios que devuelve Nova, separados de la identidad de servicio.
- consumer_generation=1 y provider generation=3 conservados en la captura.
- Usages pasaron de cero a esos recursos en el provider esperado.

Se borró la instancia por Nova, se esperó su desaparición y se comprobó
allocations vacías y usages de vuelta a cero. Las generaciones se conservan, no
se esperan decrecientes al liberar. No quedaron instancias según el listado
all-projects final. Este es un boot real de Nova, distinto del probe biblioteca B13.

## Runner y garantías

scripts/integration/placement_boot.py ejecuta el ciclo con SDK real dentro de la
VM y admin openrc. Exige --allow-mutations, output nuevo, token privado e IDs
explícitos de provider/imagen/flavor/red. Confirma que el original está detenido
y Go anuncia 1.36; verifica contenido/owner de allocations y usos. No sigue
redirects con credenciales ni serializa adminPass, tokens o el objeto Server entero.

El nombre aleatorio tt-placement-b14 identifica el único server que puede borrar;
se registra su ID inmediatamente. Si create queda incierto, busca sólo ese nombre
exacto y aborta si es ambiguo. finally verifica propiedad y ejecuta delete/wait y
lecturas posteriores. Plazos acotados; un fallo de cleanup no se convierte en PASS.
Guards rechazan ausencia de opt-in y evidencia existente antes de SDK o borrado.

Reproducción tras preparar B13 y su ruta Go, dentro de la VM:

```sh
python placement_boot.py --provider-id "$LAB_PROVIDER_UUID" \
  --image-id "$EXISTING_IMAGE_UUID" --flavor-id "$EXISTING_FLAVOR_ID" \
  --network-id "$EXISTING_NETWORK_UUID" --token-file "$PRIVATE_SERVICE_TOKEN" \
  --output "$NEW_OUTPUT" --allow-mutations
```

El cambio de ruta y restauración usan placement_switch.py, con lease automático
900s en este ensayo. El orquestador de la ejecución está archivado sin secretos;
B15 conserva convertirlo en suite integral y exigir dos ejecuciones/negativos.

## Restauración y cierre de sesión

El original volvió a anunciar 1.39 con su site exacto y unidad activa. El cliente
Nova desplegado volvió a obtener candidates del original tras restaurar. Provider,
inventarios, traits, aggregates, allocations y UUID de su MySQL coinciden con el
preflight anterior; el perfil AppArmor original conserva su SHA. Keystone, Glance,
nova-compute y scheduler estaban activos; la imagen original seguía ACTIVE.

Se pararon las unidades Go/MySQL propias y se eliminó su datadir, schema/cuentas,
perfil AppArmor independiente, DSN y archivos privados. Se eliminó el usuario/
proyecto de prueba de Keystone con sus app credentials y directorio privado. No
quedaban unidades/perfiles activos tt-placement. **tt-openstack-lab quedó apagada.**
La evidencia final resuelve los recursos que B13/B16 mantenían en uso para B14.

make check pasó con guards, harness/infra y vet/race/build Placement/Glance. No se
modificó Nova, Excel, S2 ni se ejecutó SQL de escritura en Placement original.
B15 sigue pendiente, al igual que las campañas B17/B18. Glance original sirvió la
imagen; este ensayo no cierra B26 ni demuestra toda la API Placement, nested/sharing,
HA, boot por red/SSH de la instancia o resultados comparativos de rendimiento.

Cierre posterior: [B15](b15-integration.md) automatiza preparación, casos positivos/
negativos y restauración; sus dos corridas finales pasan 11/11. B17/B18 conservan
las campañas de rendimiento independientes.
