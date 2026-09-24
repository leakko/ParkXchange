---
name: brainstorming
description: >-
  MUST use before any new feature, behaviour change, or non-trivial UI.
  Interview the user (one question at a time), explore context, propose
  approaches, write a short design, get approval — then hand off to writing-plans.
  Never write implementation code until the user approves the design.
---

# Brainstorming (diseño antes de código)

**HARD-GATE:** no escribas código de producción, scaffolds ni “empiezo ya” hasta
tener un diseño aprobado por el usuario. Si el cambio es trivial (typo, un string,
fix de una línea con causa clara), dilo y salta a implementación + TDD.

## Flujo

1. Lee `PROGRESS.md` y el código / specs relacionados (`docs/superpowers/specs/` si existen).
2. Resume en 2–3 líneas lo que crees que pide.
3. Pregunta **una sola cosa** por turno (objetivo, usuario, casos borde, no-goals).
4. Cuando tengas suficiente: propone **2–3 enfoques** con trade-offs y una recomendación.
5. Presenta el diseño por secciones (comportamiento, datos/API, UI, riesgos). Pide OK.
6. Si el alcance lo merece, guarda
   `docs/superpowers/specs/YYYY-MM-DD-<tema>-design.md`.
7. Tras aprobación → carga la skill `writing-plans` (no implementes aún).

## Qué aclarar siempre en ParkXchange

- ¿Regla de negocio en use case / domain, o solo UI?
- ¿Afecta mapa, intercambio, push, ubicación background, auth?
- ¿Migración SQL / contrato API / i18n es+en?
- Criterio de “hecho” observable (demo / test).

## Anti-patrones

- “Es simple, voy directo al código.”
- Preguntas en bloque de 10.
- Diseñar sin mirar `AGENTS.md` / capa hexagonal.
