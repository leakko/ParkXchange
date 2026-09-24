---
name: writing-plans
description: >-
  Use when requirements or a design exist and you must create an exhaustive
  implementation plan BEFORE any production code. Bite-sized TDD tasks, file map,
  risks, verification commands. Save under docs/superpowers/plans/.
---

# Writing plans

Tras un diseño aprobado (skill `brainstorming` o spec existente), escribe un plan
ejecutable. **No toques código de producción** mientras redactas el plan.

## Destino

`docs/superpowers/plans/YYYY-MM-DD-<feature-name>.md`

## Contenido obligatorio

1. **Goal** y out-of-scope.
2. **File map** — qué archivos crear/cambiar y por qué (capa: domain / use case / adapter / mobile).
3. **Tasks** numeradas, cada una:
   - Objetivo
   - Archivos
   - Pasos TDD: test que falla → implementación mínima → verde
   - Comando de verificación (`task api:test`, tsc, etc.)
4. **Riesgos** (concurrencia PostGIS, permisos ubicación, i18n, Play, etc.).
5. **Done when** — demo o checklist observable.

## Estilo de tareas

- Una tarea = un ciclo testable pequeño.
- Respeta hexagonal: nada de reglas en handlers.
- YAGNI: no inventes abstracciones “por si acaso”.
- Asume que quien ejecuta no conoce el repo: rutas concretas, no “actualizar el servicio”.

## Cierre

Muestra el plan, pide confirmación si hay decisiones abiertas, luego ejecuta tarea
a tarea con `test-driven-development` y al final `verification-before-completion`.
