---
name: test-driven-development
description: >-
  Use when implementing any feature or bugfix. Write a failing test FIRST,
  watch it fail, then minimal production code. No production code without a
  failing test first (except pure config/docs with user OK).
---

# Test-Driven Development

## Ley

```
NO PRODUCTION CODE WITHOUT A FAILING TEST FIRST
```

Si ya escribiste implementación sin test: bórrala o déjala de lado y empieza por el test.

## Dónde va el test (ParkXchange)

| Qué | Dónde |
| --- | --- |
| Regla pura / transición | `services/api/internal/domain` |
| Decisión de use case | paquete del servicio + fake `Store` |
| Índice / constraint / race | integración PostGIS real |
| UI / hook puro | test junto al módulo mobile si ya hay patrón |

## Ciclo por tarea del plan

1. Escribe el test que describe el comportamiento deseado.
2. Ejecuta y **confirma el fallo** (mensaje esperado).
3. Implementación mínima para pasar.
4. Ejecuta de nuevo → verde.
5. Refactor solo con tests verdes.
6. Siguiente tarea del plan.

## Excepciones (pide OK al usuario)

- Prototipo throwaway
- Solo config / copy / assets
- Generado por herramienta

## Anti-patrones

- Test escrito *después* “para cubrir”.
- Test que solo aserta mocks sin comportamiento.
- Debilitar `arch_test` o saltarse capas para que “compile el test”.
