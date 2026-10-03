# Informes individuales de Etapa 2 / Sprint 2

Fecha del informe y de la línea base: 2 de octubre de 2026.

| Módulo | Estudiante | Fuente editable | PDF de entrega |
|---|---|---|---|
| Placement | Paolo Castañon Barrera | [LaTeX](informe_etapa2_placement.tex) | [PDF](../../../output/pdf/informe_etapa2_placement_Paolo_Castanon_Barrera.pdf) |
| Glance | Juan Cardenas Uribe | [LaTeX](informe_etapa2_glance.tex) | [PDF](../../../output/pdf/informe_etapa2_glance_Juan_Cardenas_Uribe.pdf) |

Ambos informes toman como referencia visual `informe_etapa1_openstack_Placement_Glance.pdf`, proporcionado por el usuario. Conservan portada institucional, tipografía Latin Modern, formato A4, índice, seis secciones principales, cuadros, encabezados y numeración. Cada PDF tiene 20 páginas físicas: portada y 19 páginas numeradas.

El contenido de Etapa 2 procede de la documentación y las capturas de `docs/sprint2/` y `evidence/sprint2/`, con los identificadores del Excel vigente. Los resultados comunes de infraestructura se identifican como compartidos; no se atribuyen puntos ni ejecución individual a partir de esas pruebas. Las propuestas Go y sus benchmarks se distinguen de las pruebas ejecutadas sobre los servicios originales. B20 y B25 de Glance conservan su planificación en S4; la sustitución de Glance y el arranque de Nova con el servicio Go (B26) se mantienen en S5.

## Ajustes según la pauta de Etapa 2

Cada informe incorpora la arquitectura propuesta del servicio Go, el modelo entidad-relación, una secuencia de interacción con Nova y la relación entre requisitos, prácticas de calidad y pruebas previstas. Las decisiones describen alternativas, fundamentos y consecuencias; la planificación documenta commits y cambios respecto del plan inicial. Los diagramas distinguen las observaciones de la línea base y el comportamiento propuesto.

La comparación cubre las 26 tareas y 185 puntos del backlog vigente. El diseño principal de Sprint 2 comprende B04, B05, B06 y B19, completadas (4/4 tareas, 20/20 puntos). El avance global formal es de 7/26 tareas y 33/185 puntos. B21 continúa en progreso y B16 pendiente según el Excel; sus estados no se modificaron. El cierre del diseño principal de Sprint 2 no implica que todo el diseño del proyecto esté terminado.

## Reconstrucción

Las fuentes son documentos independientes, sin archivos auxiliares del proyecto. Requieren una distribución TeX con pdfLaTeX, latexmk, Babel, Latin Modern, microtype, geometry, booktabs, longtable, enumitem, titlesec, fancyhdr, lastpage, TikZ, needspace, placeins y hyperref. Babel usa `babelprovide` para la configuración española.

Desde la raíz del repositorio:

```bash
REPORT_BUILD_DIR=$(mktemp -d)
latexmk -pdf -interaction=nonstopmode -halt-on-error \
  -outdir="$REPORT_BUILD_DIR" docs/sprint2/reports/informe_etapa2_placement.tex
cp "$REPORT_BUILD_DIR/informe_etapa2_placement.pdf" \
  output/pdf/informe_etapa2_placement_Paolo_Castanon_Barrera.pdf
latexmk -pdf -interaction=nonstopmode -halt-on-error \
  -outdir="$REPORT_BUILD_DIR" docs/sprint2/reports/informe_etapa2_glance.tex
cp "$REPORT_BUILD_DIR/informe_etapa2_glance.pdf" \
  output/pdf/informe_etapa2_glance_Juan_Cardenas_Uribe.pdf
```

Los auxiliares se generan fuera del repositorio. El compilador integrado no pudo descargar su bundle TeX durante esta elaboración; los PDF de entrega se generaron correctamente con pdfLaTeX local. Se revisaron todas las páginas renderizadas y se verificaron autor, tamaño A4, índice, referencias y ausencia de desbordamientos de maquetación.

La VM permaneció apagada durante la elaboración. Este trabajo no requiere repetir instalación ni pruebas de OpenStack.
