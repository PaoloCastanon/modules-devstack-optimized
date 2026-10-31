# Preparación y pilotos B17/B18 — 2026-10-30, America/Santiago

No son mediciones oficiales. B16 conserva su matriz de 240 ventanas de 30 s de
warm-up y 120 s de carga. B13/B14/B15 están integrados en base 2046a95. Excel y S2
permanecen intactos; Glance original es control.

- Pilotos05 y06: **24/24 ventanas cada uno**, las seis cargas en Python/Go y
  concurrencias1/16, warm-up1s y carga4s. Cero errores HTTP/estado, semillas
  equivalentes por par y limpieza completa. Análisis CPU/PSS también válido24/24
  en ambos, sin errores de membresía, relojes, PSS ni reposo. No se agregan para
  simular las cinco repeticiones ni la duración de una campaña oficial.
- Piloto06 usa exactamente los hashes finales de placement_window.py,
  placement_campaign.py, placement_lab.py y placement_resources.py. Los manifests
  registran fuente, protocolo, semilla completa, orden y configuración efectiva.
- Preparaciones01/02 detectaron PUT415 y03 PUT400: faltaba Content-Type y se
  pedía1.2 para crear una clase en vez de1.7. Se detuvieron y limpiaron; no cuentan
  como éxito. No se cambió el servidor para aceptar requests incorrectas.
- Piloto04 completó dos ventanas HTTP pero falló al limpiar con DELETE provider
  1.36, fuera del perfil Go1.20. La ruta original se restauró y se retuvo el
  backend/ledger/credenciales. Reparación acotada con DELETE1.0 y verificación de
  nombres/owner retiró esos recursos. El manifest histórico conserva cleanupfalse;
  pilot-04-repair.log y su cleanup.json posterior registran la reparación efectiva.
  No se reclasifica esa corrida como aprobada. Directorio privado host también
  retirado después de comprobar la reparación.
- Piloto06 final: original providers/inventarios/traits/allocations y owners SQL
  iguales; hash Apache y perfil AppArmor MySQL original intactos; cinco servicios
  originales activos; backend, identidad y raíces privadas propios retirados.
  La VM permanece temporalmente encendida para la campaña oficial inmediatamente
  posterior; su apagado se registrará al terminar esa sesión.
- make-check.log PASS: 52 tests Python de unidad, harness/infra y vet/race/build
  de ambos dominios. Cuatro tests B17 y tres B18 incluyen sockets HTTP reales,
  claim204 sin persistencia rechazado, limpieza por propiedad, generaciones/tipos,
  membresía/PSS/CPU sobre ventana común y bloqueo de publicación parcial.
  b18-shutdown-red/green reproduce y corrige la aceptación incorrecta de una lista
  vacía de servicios como prueba de restauración; no se apagó la VM en el test.

Datos crudos conservados en JSONL gzip, sin tokens/headers de autenticación,
contraseñas, DSN ni stderr del canal RPC privado. La fuente inicial disponible
se archiva en source-history cuando su hash coincide con el primer manifest;
los cambios intermedios llevan sus hashes en cada manifest. El binario Go es el
mismo20b7b92d… de B15. Las dos bases MySQL son independientes; se declaran sus
buffer pools y conexiones diferentes, los dos workers Python y las cachés auth.
Esto prepara comparaciones de despliegues, no una atribución exclusiva al lenguaje.

Reproducción: docs/benchmarks/campaign-runbook.md. Los summaries conservan
b17_accepted=false y b18_accepted=false porque su stage es pilot y sólo hay24
ventanas por campaña, frente a las240 oficiales requeridas.
