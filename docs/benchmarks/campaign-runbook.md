# Campaña B17/B18 de Placement

Instrumentación posterior a B15, fecha local 2026-10-30. El protocolo B16 revisión
1 permanece congelado: seis escenarios × cuatro concurrencias × cinco pares
Python/Go = **240 ventanas** de 30 s de warm-up y 120 s de carga. Son diez horas
de ventanas, más preparación, limpieza, muestreo en reposo y cambios de ruta.
Los pilotos no son resultados oficiales ni completan B17/B18.

## Preparación y permisos

Ejecutar desde el host, con tt-openstack-lab ya iniciada, DevStack disponible,
SSH stack y sudo del laboratorio. No ejecutar DevStack en el host. El coordinador
usa el SDK desplegado y openrc admin dentro de la VM. Preparar un directorio
nuevo de código público dentro de la VM con la misma estructura de scripts/
(benchmarks/, integration/ y placement_differential.py) y un binario Placement
compilado con `go build -trimpath -buildvcs=false`, llamado placement en su raíz.
Registrar `go version -m`, hashes, XML libvirt y configuración del host.

El comando exige --allow-mutations y directorios nuevos. Adquiere el mismo lock
exclusivo de B15. Sólo admite un laboratorio sin instancias. Crea identidad y
backend Go/MySQL propios. Las application credentials de este ensayo duran 24 h,
con tokens renovados fuera de las ventanas al inicio de cada par; el valor por
defecto de los otros ensayos sigue siendo 8 h.

La semilla completa y el orden aleatorio con semilla 20261029 se guardan antes
de la primera mutación. Cada par tiene 16 UUID de providers/consumers y una clase
CUSTOM_TT_B17 exclusiva; ambos servicios reciben los mismos IDs y cantidades.
El owner de allocations reutiliza project/user presentes en Placement Python;
se verifican sus filas mediante SELECT antes y después. No hay SQL de escritura
en la base original. La clase se crea con PUT 1.7, no con el PUT de renombrado
1.2–1.6; los providers se eliminan con DELETE 1.0.

## Ejecución

Ejemplo de piloto (cambiar todos los directorios de resultados/estado en cada
ejecución; --remote-code identifica el directorio público ya desplegado):

```bash
python3 scripts/benchmarks/placement_campaign.py \
  --pilot --allow-mutations \
  --protocol docs/benchmarks/protocol-v1.json \
  --output /tmp/b17-pilot-N \
  --private-root /tmp/tt-placement-b17-private-N \
  --remote-code /home/stack/tt-placement-b17-code \
  --remote-work /home/stack/tt-placement-b17-work-N \
  --remote-output /home/stack/b17-result-N \
  --ssh-key /home/paolo/.ssh/id_ed25519 \
  --known-hosts infra/lab-vm/.cache/known_hosts
```

El piloto usa las seis cargas, concurrencias 1/16 y un par por celda, con 1 s de
warm-up y 4 s de carga. Para la campaña oficial omitir --pilot y usar directorios
nuevos: se aplican los parámetros B16 completos, sin reducir repeticiones.
`make placement-benchmark PLACEMENT_BENCHMARK_ARGS='...'` invoca el mismo comando.

El generador usa una conexión y un consumer/provider por worker, reloj monotónico
y una barrera común. Valida cuerpo/status, cantidades, owner y generaciones.
No inicia requests después del deadline; drena la solicitud pendiente y limpia
ciclos parciales fuera de cada ventana. GET candidates en la mezcla valida el
conjunto exacto y permite únicamente usos VCPU/CUSTOM 0/1 correlacionados por
provider, que son los estados posibles de claims concurrentes. No borra ni
normaliza generaciones para ocultar diferencias. Guarda cada solicitud, incluidos
errores/timeouts, y comprime JSONL sin descartar observaciones.

Cada corrida lleva muestreo a 1 Hz desde cinco segundos de reposo hasta después
del drain. Enumera todos los PIDs del cgroup y PID+start_ticks, CPU, PSS y RSS.
Lee smaps como root, sin argumentos/environment de procesos. MySQL de ambos
servicios, Apache, Keystone, compute, scheduler y Glance tienen grupos de control
separados. Se registra CPU del recolector y del generador, incertidumbre de relojes
y duración de cada lectura. El binario Go no expone heap/GC: se declara esa ausencia
y se mide PSS total. Los defaults registrados de las bibliotecas Python se leen
del entorno desplegado; la configuración seleccionada se guarda sin secretos.

## Análisis y cierre

```bash
python3 scripts/benchmarks/analyze_placement.py /ruta/campaña \
  --output /ruta/nueva/summary.json
python3 scripts/benchmarks/publish_placement_results.py \
  --summary /ruta/nueva/summary.json --output /ruta/nueva/publicacion
```

El análisis cruza recuentos crudos y reports, calcula percentiles por operación,
goodput, variación y ratios de pares. Bootstrap de medianas, 10000 remuestras y
95% de confianza, con repetición independiente como unidad. No promedia percentiles
para presentarlos como percentiles globales. p99 con menos de 1000 respuestas
válidas se marca insuficiente. CPU usa sólo muestras interiores a la carga y
requests correctas completadas en ese mismo intervalo; comprueba membresía también
antes/después. Falta de PSS, cambio de PIDs, huecos o ausencia de reposo invalidan
B18. Datos del recolector/control no se atribuyen a Placement.

Sólo se publica aceptación con las 240 corridas válidas y restauración/limpieza.
Una excepción conserva evidencia y detiene la campaña. Si falla limpiar, mantiene
ledger/backend/credenciales para reparación; no elimina los medios necesarios
para resolverla. Restaura la ruta original también si falla borrar la semilla;
un timer de 600 s protege cada sustitución. Al terminar verifica providers,
inventarios, traits, allocations, owners SQL, hash Apache y perfil MySQL original,
servicios originales activos, retirada del backend e identidad propios y archivos
privados. Apagar la VM después de comprobar la restauración; no apagarla mientras
se necesita reparar un fallo. No guardar el canal SSH privado ni stderr/credenciales.

Las diferencias de workers, caching de tokens, pools y buffer pool MySQL deben
acompañar el informe: la comparación es de estos despliegues y no demuestra una
causa exclusiva del lenguaje. Glance original permanece como control; no cierra B26.
