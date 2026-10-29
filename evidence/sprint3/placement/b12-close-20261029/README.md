# B12 — cierre de Keystone y políticas

2026-10-29; base B11 19854f7 más las fuentes identificadas en source-files.sha256.
Los reportes conservan respuestas crudas, estado antes/después, diferencias y
cleanup. No contienen tokens, secretos, DSN ni archivos privados.

- attempt-1: 99/134; 29 diferencias de representación de unscoped, tres headers
  de inventory individual y tres expectativas erróneas del runner (GET trait 204).
  No se cuenta como PASS. Revocación/validez/expiración ya pasaban 3/3 cada una.
- attempt-2: políticas 134/134, revocación 3/3, trust válido 3/3, expirado 3/3.
  Total 143/143; todas las limpiezas PASS. fail-closed.json añade tres verificaciones
  sólo Go con hash de DB intacto y recuperación de credenciales.
- regressions: B07 57/57, B08 82/82, B09-A 60/60, B09 moderno 24/24, B10 44/44;
  total 267/267. Seeds sin diferencias y reinicio Go persistente comprobado.
- durability-final: B11 9/9 sobre el binario corregido; dump completo idéntico,
  SIGKILL Go/MySQL, rollback de owner sin commit y restore en otra cuenta.
- mysql-final: snapshots concurrentes B10 y timestamp individual B12 con dos
  clases diferentes, más filtro de clase parametrizado. make-check.log: PASS.
- red-tests.log reproduce los errores antes de corregir; green-tests.log y
  make-check.log registran la solución. La suite incluye guards del runner.

identities.json guarda sólo metadata de las identidades reales, incluida Nova
extraída de nova.conf. La identidad revocable se reemitió antes del segundo
intento; cada intento guarda su propia expiración/revocación. Los tokens de trust
usan un plazo real de 25 segundos. El original no conoció el token revocable antes
de la revocación: no se afirma invalidación inmediata de su caché positiva.

original-cleanup.json consulta la DB original sólo para contar IDs propios de
owned-original-ids.json: cero filas, proyectos/usuarios globales iguales al preflight,
sin incomplete IDs. trust-cleanup.json e identity-cleanup.json verifican eliminación
de trusts/trustee, usuario/proyecto/application credentials y revocación del token
Nova recién emitido. lab-final-state.json registra VM apagada y ausencia de
contenedores/volúmenes propios. El resto de limpieza está en cada report.

Reproducción del comparador público (tokens privados admin/service/reader/member/
unscoped/nova en TOKEN_DIR; usar un directorio nuevo por fase):

```sh
python3 scripts/placement_auth_differential.py \
  --python-url "$ORIGINAL_PLACEMENT_URL" --go-url "$GO_PLACEMENT_URL" \
  --project-id "$EXISTING_PLACEMENT_OWNER_PROJECT" \
  --user-id "$EXISTING_PLACEMENT_OWNER_USER" --tokens-dir "$PRIVATE_TOKEN_DIR" \
  --phase policies --output "$NEW_OUTPUT" --allow-mutations
```

Las otras fases son revoked, valid-expiring y expired (archivos revoked.token y
expired.token). Se revoca/reemite mediante Keystone real, no alterando el reloj
ni inventando tokens. El owner de allocations reutiliza metadata Placement
existente para no crear filas globales sin endpoint de eliminación.

reproduction/ archiva los helpers exactos del laboratorio, con rutas/host de esta
estación: identity-sdk.py.txt prepara una identidad nueva, auth-sdk.py.txt añade roles,
Nova y trusts; host-campaign.py.txt aísla MySQL/binario y ejecuta fases/fallos;
strip-prefix-proxy.go.txt sólo adapta /placement; original-readonly-proof.py.txt verifica
IDs propios. Antes de reutilizarlos ajustar rutas, generar otra identidad/root
privado y preflight, y apuntar /tmp/b09-keystone.py y /tmp/b12-keystone.py a las
copias SDK. No reutilizar IDs o secretos históricos ni ejecutar cleanup contra
cuentas ajenas. Los SDK usan el entorno admin de openrc dentro de la VM y sus
librerías instaladas; no se imprimen credenciales de nova.conf. El comparador
público exige opt-in, tokens privados y output nuevo. B11 se reproduce con su
script público y un volumen/schema propios.

Ver docs/placement/b12-acceptance.md para contrato y límites: alcance parcial,
sin cambiar Nova/endpoints ni acreditar B13–B15 o boot ACTIVE.
