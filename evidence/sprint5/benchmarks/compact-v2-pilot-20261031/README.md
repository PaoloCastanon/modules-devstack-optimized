# Piloto Placement compacto v2, 2026-10-31

24/24 ventanas HTTP válidas y 24/24 con muestras CPU/PSS válidas; seis cargas,
concurrencias 1/16, un par por celda, warm-up 1s/carga 4s. Sin errores de análisis.
Datos completos comprimidos y fuentes identificadas en manifest.json. Este
piloto comprueba instrumentos/controles; no acredita la campaña oficial de 72
ventanas (tres pares y 10/30s). pilot-summary.json mantiene ambos accepted=false.

maintenance-controls-before/after y cleanup en lab acreditan mantenimiento
suspendido temporalmente, límites journal/logs y restauración de sus estados.
La guardia anfitriona se registra por ventana. Original/owner/SQL/perfil Apache
comprobados, backend/identidad/estado privado retirados y VM apagada normalmente
(vm-final-state.json). Logs operacionales retenidos privadamente en la VM;
las muestras de carga de esta carpeta se conservan completas.
