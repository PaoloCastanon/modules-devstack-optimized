# Campaña conjunta Placement × Glance con Nova — oficial joint-compact-v4

2026-11-28, 02:21–03:01 UTC. Código `8b3e48a`, binarios `glance`
`652a7337…` y `placement` `91866d8f…`. VM `tt-openstack-lab` (4 vCPU,
8 GiB, qemu), flavor `m1.nano`, CirrOS 0.6.3 y una imagen nueva por boot (en
frío). [Protocolo](../../../../docs/benchmarks/joint-protocol-compact-v4.json) ·
[runbook](../../../../docs/benchmarks/joint-campaign.md).

**Resultado: 60/60 bloques válidos, 140 boots, limpieza completa**
(`report.json`). Primer intento oficial. Amplía la v3 (24 bloques) con 4 boots
a la vez, 5 repeticiones, desglose por fase, CPU por boot, PSS medio y PSS en
reposo. Tablas: [analysis.md](analysis.md).

## Lectura (mediana [mín–máx] de 5 repeticiones)

- **Tiempo de boot:** igual en las cuatro variantes. POST→build fue 1,9–3,1 s
  con 1 boot, ~3,6 s con 2 y ~5,3 s con 4. Los cocientes frente a
  python-python quedan en 0,92–1,05, con rangos que cruzan 1. El 80 % del boot
  es el spawn en libvirt/qemu. Con 1 boot, el spawn es bimodal (~1,3 s o
  ~2,6 s) en todas las variantes; es ruido del hipervisor, no de los servicios.
- **Fases donde intervienen los servicios:**
  - POST→cómputo (API, scheduler y Placement): 0,28 s con Placement Go frente
    a 0,32 s con Placement Python (1 boot). Con más boots se iguala.
  - Descarga de la imagen (lock del archivo base): 0,22–0,23 s con Glance Go
    frente a 0,26–0,27 s con Glance Python (1 boot).
- **Tiempo del lado del servidor** (trazas, mediana por boot):
  - Placement: 17 ms en Go frente a 102 ms en Python. Solo candidatos + claim:
    15,5 ms frente a 67,5 ms.
  - Glance: 42,5 ms frente a 109 ms. Solo la descarga de la imagen: 28 ms
    frente a 80 ms.
- **CPU por boot:**
  - Placement: ~0,006 s en Go frente a 0,06–0,08 s en Python (≈ 0,08–0,10×).
  - Glance: ~0,11 s en Go frente a 0,13–0,20 s en Python. Glance sirve los
    ~21 MiB de la imagen en cada boot.
  - Pila Placement+Glance, go-go frente a python-python: 0,42–0,56×.
- **Memoria (PSS medio, igual en reposo):**
  - Placement: ~14,5 MiB en Go frente a ~180 MiB en Python.
  - Glance: ~17–19 MiB en Go frente a ~237 MiB en Python.
  - Pila: ~32 MiB en go-go frente a ~417 MiB en python-python (0,076×). No
    cambia con 1, 2 o 4 boots.
- **Peticiones:** las mismas en las cuatro variantes (Placement 6/14/~35,
  Glance 6/12/24 para 1/2/4 boots). Nova ve el mismo protocolo.

## Límites

- 5 repeticiones: mediana y rango, sin significación estadística.
- Las fases salen del journal de nova-compute (µs) y de la hora de POST en la
  misma VM. POST→ACTIVE se mide por polling cada 1 s; el retardo del polling se
  publica aparte.
- La descarga se mide como el tiempo que nova-compute retiene el lock del
  archivo base: incluye traer la imagen desde Glance y convertirla con qemu-img.
- El tiempo del lado del servidor de uWSGI (Python) tiene resolución de 1 ms.
