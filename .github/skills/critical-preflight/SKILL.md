---
name: critical-preflight
description: >-
  Use near the end of a feature or before shipping. Hunt critical bugs and
  regressions: authz holes, layering violations, concurrency, location privacy,
  missing tests, broken contracts. Produce a short risk list with severity.
---

# Critical preflight

Ejecuta esto **antes** de declarar la feature terminada (después de tests verdes).

## Checklist ParkXchange

1. **AuthZ / ownership** — ¿alguna acción sin comprobar actor en el use case?
2. **Layering** — ¿lógica en handler? ¿adapters importándose? ¿HTTP status en use case?
3. **Concurrencia / DB** — claims, uniques parciales, sweeps: ¿sigue siendo un write atómico?
4. **Ubicación / privacidad** — ¿Always solo tras «Voy de camino»? ¿datos exactos solo tras revelar?
5. **Contratos** — API/WS/mobile desalineados; i18n incompleto.
6. **Estados de intercambio** — cancel, expire, en_route, ready: ¿regresiones?
7. **Tests** — ¿falta el caso que habría pillado el bug más caro?
8. **Ops** — ¿migración necesaria? ¿deploy solo de API basta?

## Salida

Lista breve:

```
P0: ...
P1: ...
P2: ...
OK / residual risks: ...
```

Si hay P0, **no** digas que está listo; arregla o escala al usuario.
Luego pasa por `verification-before-completion`.
