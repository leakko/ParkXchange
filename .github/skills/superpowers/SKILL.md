---
name: superpowers
description: >-
  Rigorous mode for multi-part or risky changes. Incomplete requirements,
  broad systems, or "review before we call it done". Orchestrates brainstorming,
  writing-plans, TDD, critical-preflight, and verification-before-completion.
  Skip for tiny localized fixes with clear behaviour.
---

# Superpowers

Para trabajos complejos, piensa antes de editar y valida antes de cerrar. No
conviertas esto en burocracia para un typo o un fix localizado ya claro.

## Flujo (orquesta otras skills)

1. Requisitos incompletos → `brainstorming` / `entrevistador-procesos` (sin código).
2. Diseño OK → `writing-plans` (plan en `docs/superpowers/plans/`).
3. Cada tarea → `test-driven-development`.
4. Antes de cerrar → `critical-preflight` luego `verification-before-completion`.
5. Respeta `AGENTS.md` y capas; diffs mínimos; evidencia real de comandos.

## Criterios de calidad

- El cambio resuelve la causa raiz y no solo el sintoma.
- Las reglas de negocio permanecen en la capa apropiada.
- Los contratos publicos y cambios del usuario se conservan.
- Hay una comprobacion ejecutable del comportamiento modificado.
- Se explican los supuestos, riesgos y tests que no pudieron ejecutarse.

## Formato de trabajo

Comunica brevemente entendimiento, plan cuando aporte valor, progreso, validacion y resultado final. Pregunta antes de actuar solo si falta una decision critica; si no, usa una suposicion razonable y dejala explicita.
