# Campaña conjunta Placement × Glance (joint-compact-v3)

2026-11-27. Primera campaña que mide **Placement Go y Glance Go funcionando
juntos** con Nova original, frente a las otras tres combinaciones. Protocolo:
[joint-protocol-compact-v3.json](joint-protocol-compact-v3.json) (misma matriz
que la compact-v2, que nunca tuvo runner).

## Diseño

| Variante | Placement | Glance |
|---|---|---|
| `python-python` (referencia) | Python | Python |
| `go-python` | Go | Python |
| `python-go` | Python | Go |
| `go-go` (objetivo) | Go | Go |

- Lotes de 1 y 2 boots concurrentes, 3 repeticiones: **24 bloques, 36 boots**.
  Piloto: 1 repetición, **8 bloques, 12 boots** (no cuenta como oficial).
- Orden con semilla 20261127. En cada repetición las cuatro variantes van en
  orden aleatorio y, dentro de cada una, las concurrencias también. Así se
  cambian las rutas cuatro veces por repetición, no en cada bloque.
- Al cambiar de variante: se vuelven ambas rutas a Python (renueva los leases
  de rollback), se activa la variante, se reinician `n-cpu` y `n-sch`, se
  espera a que Nova pueda programar y a ver el inventario del compute en el
  Placement activo, sin uso, y se dejan 10 s de reposo.
- **Caché fría por boot:** cada servidor usa una imagen CirrOS propia y nueva
  (UUID determinista por run/bloque), subida por el Glance activo antes de la
  ventana. La entrada `_base/sha1(image_id)` de Nova debe faltar antes y
  existir después; el Glance activo debe servir la descarga completa dentro
  de la ventana.
- Ventana: desde la barrera común hasta el último servidor `ACTIVE` (+3 s de
  vaciado de logs). Polling de cada servidor a 1 s (±1 s), plazo 300 s.

## Qué valida cada bloque

1. Todos los servidores `ACTIVE`, con su imagen.
2. Allocation de cada servidor en el Placement activo, igual al flavor.
3. Trazas dentro de la ventana:
   - Placement activo: `GET /allocation_candidates 200` y un
     `PUT /allocations/{id} 204` por servidor;
   - Glance activo: una descarga `GET /v2/images/{id}/file 200` completa por
     imagen (uWSGI también trae el ID, que se coteja);
   - la implementación inactiva de cada servicio: **cero** peticiones (el Go
     ocioso no recibe nada; el original tras una ruta Go está detenido).
4. Tras borrar: allocations liberadas, proveedor sin uso, imágenes 204 → 404 y
   entradas `_base` propias eliminadas (sólo las de los IDs del bloque).
5. Muestreo de recursos con soporte antes y después de la ventana.

El primer bloque inválido detiene la campaña. La limpieza de la suite B26 corre
siempre: rutas y servicios originales, catálogo de Glance y Placement de
control iguales al inicio, runtimes Go, identidad temporal y validador
retirados, controles de mantenimiento restaurados.

## Métricas

Por boot: segundos `POST /servers` → `ACTIVE`. Por bloque: makespan, boots/min,
peticiones y tiempo de servidor de Placement y Glance, CPU (s) y PSS máximo por
cgroup (Placement y Glance de las dos implementaciones, MySQL propio de
Placement Go, servicios Nova, Keystone, MySQL, Apache, RabbitMQ) y CPU ocupada
de la VM. `scripts/benchmarks/cgroup_sampler.py` corre como root a 1 Hz durante
toda la campaña; cada bloque toma la diferencia entre la muestra anterior y la
posterior a su ventana.

## Ejecución

Dentro de la VM, con un directorio de código nuevo que tenga `scripts/`,
`docs/benchmarks/` y los binarios estáticos (`CGO_ENABLED=0 go build -trimpath`):

```sh
source /opt/stack/devstack/openrc admin admin
# Python del venv de DevStack: el python3 del sistema no tiene keystoneauth1/pymysql
/opt/stack/data/venv/bin/python3 scripts/integration/joint_campaign.py \
  --protocol docs/benchmarks/joint-protocol-compact-v3.json --pilot \
  --flavor-id 42 --network-id <red private> \
  --binary bin/glance --placement-binary bin/placement \
  --work-root /home/stack/tt-placement-joint-pilot-<N> \
  --output /home/stack/joint-pilot-result-<N> --allow-mutations
```

Para la oficial: sin `--pilot` y directorios nuevos. Análisis (en la VM o en el
host, sobre la salida copiada):

```sh
python3 scripts/benchmarks/analyze_joint.py <salida>/blocks.jsonl \
  --json <salida>/analysis.json --markdown <salida>/analysis.md
```

Salida: `manifest.json` (plan, hashes de binarios y de CirrOS, protocolo),
`blocks.jsonl` (un registro por bloque con todos los boots), `samples.jsonl`,
`traces/block-NN.json` (peticiones de las cuatro unidades, sin usuarios),
`report.json` (cambios de variante, limpieza, pase). No contienen tokens.

## Requisitos previos

- `glance_go_shared` en el MySQL del original: `placement_runtime.prepare_official`
  la crea si falta (antes la suite B26 fallaba en una VM donde nunca se corrió
  `glance_replace.py install`).
- VM sin servidores; ≥ 8 GiB libres (cada boot frío deja ~112 MiB en `_base`
  hasta el final del bloque).

## Límites

- Una VM de 4 vCPU con Nova `LIBVIRT_TYPE=qemu`: el tiempo de boot lo domina
  libvirt/qemu y la conversión de la imagen, no Placement ni Glance. La campaña
  mide si la sustitución cambia el boot de punta a punta y cuánto cuestan los
  servicios dentro de esa ventana, no el rendimiento de las API aisladas (eso
  ya lo miden las campañas de Placement y de Glance).
- Tres repeticiones por celda: mediana y rango, sin percentiles de cola ni
  significación estadística.
