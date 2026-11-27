# Campaña conjunta Placement × Glance con Nova — oficial joint-compact-v3

Fecha local 2026-11-27, 22:22–22:47 UTC. Código `1b74b3d`, binarios
`glance` `652a7337…` y `placement` `91866d8f…` (los del piloto), protocolo
sha256 `232f06d4…`. VM `tt-openstack-lab` (4 vCPU, 8 GiB, qemu), flavor
`m1.nano`, CirrOS 0.6.3, imagen nueva por boot.
[Protocolo](../../../../docs/benchmarks/joint-protocol-compact-v3.json) ·
[runbook](../../../../docs/benchmarks/joint-campaign.md).

**Resultado: 24/24 bloques válidos, 36 boots, limpieza completa**
(`report.json`): rutas y servicios originales restaurados, catálogo de
Glance y Placement de control iguales al inicio, runtimes Go, identidad y
validador retirados. Primer intento oficial; sin intentos descartados.

## Tablas

Ver [analysis.md](analysis.md) (mediana [mín–máx] de 3 repeticiones por celda).

Lectura:
- **Boot de punta a punta:** las cuatro variantes quedan en 3,4–4,9 s de
  mediana. Los cocientes frente a Python+Python están en torno a 1 (go-go:
  0,99 con 1 y con 2 boots), con rangos que cruzan 1. El boot lo domina
  libvirt/qemu; sustituir los servicios no lo empeora.
- **Peticiones:** idénticas en las cuatro variantes (Placement 6/14,
  Glance 6/12 por lote de 1/2): Nova ve el mismo protocolo.
- **CPU dentro de la ventana:** Placement Go ~0,01 s frente a 0,07–0,17 s de
  Python; Glance Go 0,11–0,23 s frente a 0,22–0,27 s.
- **Memoria (PSS máximo):** Placement Go ~13–14 MiB frente a ~178 MiB;
  Glance Go ~17–19 MiB frente a ~236 MiB. Con go-go, los dos servicios suman
  ~32 MiB frente a ~414 MiB.

## Límites

- Tres repeticiones: mediana y rango, sin percentiles de cola ni significación.
- Polling a 1 s: diferencias de 1 s están en la resolución de la medida.
- La CPU de Glance incluye servir la imagen (~21 MiB) a Nova.
