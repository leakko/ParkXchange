---
name: optimizador-prompts
description: Convierte ideas desordenadas, notas o instrucciones incompletas en prompts claros y listos para usar. Usar cuando el usuario quiera mejorar, ordenar, reescribir, estructurar o crear un prompt para una herramienta de IA.
---

# Optimizador de prompts

Transforma la intención del usuario en una instruccion precisa, breve y directamente utilizable.

## Proceso

1. Identifica la herramienta o modelo destino. Si es ambiguo y cambia mucho el resultado, pregunta.
2. Extrae objetivo, contexto, tarea, restricciones, entradas, formato de salida, criterios de calidad y cosas a evitar.
3. Pregunta solo por informacion critica, como maximo tres preguntas.
4. Produce un unico prompt listo para copiar, adaptado a la herramienta.
5. Elimina texto que no cambie el resultado y no inventes requisitos criticos.

## Adaptacion tecnica

Para agentes de codigo incluye archivos o simbolos concretos, comportamiento actual, cambio deseado, limites de alcance, restricciones de dependencias y criterio de terminado.

Para imagen o video incluye sujeto, composicion, estilo, iluminacion, encuadre, formato y elementos a evitar.

Para automatizaciones incluye trigger, entradas, pasos, servicios, salida y manejo de errores.

## Formato

**Prompt optimizado:**

```text
[un unico prompt listo para usar]
```

**Cambios principales:** 3 a 5 puntos breves.

Incluye dudas opcionales solo si resolverlas mejora materialmente el resultado.
