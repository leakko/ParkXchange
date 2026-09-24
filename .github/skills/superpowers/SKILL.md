---
name: superpowers
description: Modo de trabajo riguroso para cambios complejos con varias partes, dependencias o riesgos. Usar cuando el usuario pida construir o modificar un sistema amplio, cuando los requisitos esten incompletos o cuando solicite revisar una solucion antes de darla por terminada.
---

# Superpowers

Para trabajos complejos, piensa antes de editar y valida antes de cerrar. No conviertas esta skill en burocracia para una tarea pequena y localizada.

## Flujo

1. Entiende el objetivo, alcance, usuarios, restricciones y la informacion que falta.
2. Formula una hipotesis local sobre el comportamiento y el cambio minimo que puede probarla.
3. Define un plan breve con dependencias, riesgos y casos limite.
4. Ejecuta en incrementos pequenos, respetando la arquitectura, convenciones y archivos fuera de alcance.
5. Tras cada edicion sustantiva ejecuta la validacion mas cercana disponible.
6. Revisa el resultado contra los requisitos, tests, diagnosticos y posibles regresiones.

## Criterios de calidad

- El cambio resuelve la causa raiz y no solo el sintoma.
- Las reglas de negocio permanecen en la capa apropiada.
- Los contratos publicos y cambios del usuario se conservan.
- Hay una comprobacion ejecutable del comportamiento modificado.
- Se explican los supuestos, riesgos y tests que no pudieron ejecutarse.

## Formato de trabajo

Comunica brevemente entendimiento, plan cuando aporte valor, progreso, validacion y resultado final. Pregunta antes de actuar solo si falta una decision critica; si no, usa una suposicion razonable y dejala explicita.
