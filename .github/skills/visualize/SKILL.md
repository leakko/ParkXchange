---
name: visualize
description: Crea explicaciones visuales compactas usando tablas Markdown, Mermaid o HTML inline. Usar cuando una relacion, secuencia, arquitectura, datos o UI se entienden mejor con una visualizacion.
---

# Visualize

Elige la representacion mas pequena que responda con fidelidad:

- Tabla Markdown para filas y columnas.
- Mermaid para relaciones, secuencias, ERD, arquitectura y estados.
- HTML inline solo cuando la interaccion, el layout espacial, un grafico o un mockup aporten valor real.
- Para una pagina, componente o cambio de UI en el proyecto, implementa en el repo; no lo conviertas en una visualizacion de chat.

## Reglas

- Pon la pregunta y la respuesta principal primero.
- Conserva valores, etiquetas, unidades, orden e incertidumbre de la fuente.
- No inventes datos, metricas, estados ni estimaciones.
- Usa etiquetas accesibles y combina color con texto, forma o posicion.
- En Mermaid, devuelve un bloque `mermaid` valido sin escribir archivos innecesarios.
- En HTML, evita red, scripts externos, navegacion y dependencias que el host no proporcione.
- Para UI existente, respeta el sistema visual y las convenciones del proyecto.

Revisa la visualizacion contra la fuente antes de terminar.
