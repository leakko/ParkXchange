---
description: Full feature pipeline — interview, design, plan, TDD, preflight, verify
agent: agent
---

Quiero una feature o cambio de comportamiento en ParkXchange.

Sigue este pipeline **en orden** (usa las skills del repo cuando existan):

1. `/brainstorming` o skill `brainstorming` (+ `entrevistador-procesos` si el proceso está vago): entrevista (una pregunta por vez), diseño, **espera mi OK**. No código aún.
2. Skill `writing-plans`: plan exhaustivo en `docs/superpowers/plans/`, tareas TDD. Espera OK si hay decisiones abiertas.
3. Skill `test-driven-development`: implementa tarea a tarea (test rojo → verde).
4. Skill `critical-preflight`: busca P0/P1 antes de cerrar.
5. Skill `verification-before-completion`: ejecuta tests/comandos reales y cierra con evidencia.

Respeta `AGENTS.md`, hexagonal, y `PROGRESS.md`. Diffs mínimos; sin docs extra salvo specs/planes del pipeline.

Feature / problema:
