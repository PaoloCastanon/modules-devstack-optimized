# Campaña conjunta por API: Placement y Glance a la vez (joint-api-v1)

2026-11-28. Parte A de la campaña conjunta. En cada ventana corren **al mismo
tiempo** el generador de Placement y el de Glance, contra las rutas públicas
de Apache, en las cuatro variantes (Placement Python/Go × Glance Python/Go).
De una misma ventana salen dos resultados:

- **Placement acompañado de Glance**, comparable con las campañas de Placement
  (`placement-compact-v2-reads/nova-20261120`): mismas cargas, 10 s de
  calentamiento, 30 s de medida, C=1/16 y 3 repeticiones;
- **Glance acompañado de Placement**, comparable con `glance-v7`.

Protocolo: [joint-api-protocol-v1.json](joint-api-protocol-v1.json).

## Diseño

| Par | Placement | Glance | Por qué |
|---|---|---|---|
| `nova` | nova-mix | nova-mix (show, descarga 16 MiB, list) | lo que Nova pide a cada servicio al arrancar una VM |
| `claim-download` | claim-cycle | download-16m | escrituras en Placement mientras Glance transmite bytes |
| `schedule-catalog` | candidates | catalog-show | la consulta más pesada de Placement con lecturas de metadatos en Glance |

3 pares × C=1/16 × 4 variantes × 3 repeticiones = **72 ventanas** (~2 h).
Piloto: 1 repetición con 2 s + 5 s, **24 ventanas**. En cada bloque
(par, C, repetición) las cuatro variantes van en orden aleatorio con semilla.
Cada ventana usa sus propios proveedores, consumidores, clase `CUSTOM_TT_B17_*`
e imágenes, derivados del ID de la campaña: nunca se reutiliza un ID.

## Instrumentos

- `scripts/benchmarks/joint_lab.py` (VM): compone `placement_lab.Lab` y
  `glance_lab.Lab` sin cambiarlos. Cada uno conserva su raíz privada, su
  identidad, su runtime Go y su registro de controles. Un solo
  `placement_resources.py` muestrea a 1 Hz todos los cgroups: las dos
  implementaciones de cada servicio, el MySQL de Placement Go, MySQL, Apache,
  Keystone, memcached, nova-compute, nova-scheduler y RabbitMQ.
- `scripts/benchmarks/joint_api_campaign.py` (host): congela el orden, siembra
  los dos servicios, comprueba que las rutas coinciden con la variante, lanza
  `placement_window.run` y `glance_window.run` en paralelo y exige que los dos
  sean válidos, que sus ventanas de medida se solapen al menos un 90 %, una
  deriva de reloj menor a 100 ms y la limpieza de las semillas.
- `scripts/benchmarks/analyze_joint_api.py`: por servicio y ventana calcula
  ops/s, p50/p95/p99 (por corrida y por operación, desde las muestras), CPU
  por petición correcta, PSS medio, pico y en reposo (método B18:
  `interval_summary`), controles y CPU de la VM; por celda da mediana y rango,
  y cocientes frente a python-python pareados por repetición.

## Ejecución

En la VM, un directorio con el contenido de `scripts/` y los binarios
`placement` y `glance` en su raíz. Desde el host:

```sh
python3 scripts/benchmarks/joint_api_campaign.py --pilot --allow-mutations \
  --protocol docs/benchmarks/joint-api-protocol-v1.json \
  --output <salida nueva> --private-root <privado nuevo> \
  --remote-code /home/stack/<código> --remote-work /home/stack/tt-placement-<nuevo> \
  --remote-output /home/stack/<salida VM nueva> \
  --ssh-key ~/.ssh/id_ed25519 --known-hosts infra/lab-vm/.cache/known_hosts
python3 scripts/benchmarks/analyze_joint_api.py <salida> --output <salida>/summary.json --markdown <salida>/summary.md
```

Oficial: sin `--pilot` y con directorios nuevos.

## Límites

- 3 repeticiones por celda: mediana y rango, sin significación estadística.
- Los dos generadores corren en el host, en lazo cerrado (opción A): el
  servicio acompañante más rápido genera más carga y le quita CPU al otro; la
  condición "acompañado" mide esa competencia real. Una barrera común alinea el
  inicio de las dos fases medidas y el solape se exige igualmente en cada ventana.

## Resultados

- Piloto: [joint-api-pilot-20261128](../../evidence/sprint5/benchmarks/joint-api-pilot-20261128/README.md) (24/24).
- Oficial: [joint-api-official-20261128](../../evidence/sprint5/benchmarks/joint-api-official-20261128/README.md) (72/72).
- Las implementaciones difieren en workers, cachés y pools: no se atribuye la
  diferencia solo al lenguaje.
- Las campañas individuales son la referencia "solo" y se midieron en otras
  sesiones. La de Placement usó la misma VM y el mismo código de Placement
  (sin cambios desde `9a5c18e`). La de Glance v7 se midió en otro equipo.
