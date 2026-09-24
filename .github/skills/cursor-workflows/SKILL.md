---
name: cursor-workflows
description: Enruta automaticamente workflows utiles inspirados en las skills de Cursor cuando una tarea requiere automatizacion, revision de codigo, revision de seguridad, visualizacion, shell, objetivos iterativos o personalizacion del agente. No usar para tareas simples que no necesitan ese workflow.
---

# Cursor workflows portable

Selecciona el workflow adecuado por contexto, sin pedir al usuario que nombre una skill.

## Enrutado

- **Revision de codigo:** prioriza bugs, regresiones, riesgos y tests faltantes. Para seguridad, revisa primero entrada, autenticacion, secretos, permisos y dependencias.
- **Division en PRs:** usa `split-to-prs` cuando haya que separar trabajo; propone el corte y espera aprobacion antes de crear ramas o publicar.
- **Automatizacion o workflow complejo:** define objetivo, entradas, pasos, estados de fallo y criterio de parada antes de ejecutar.
- **Visualizacion o UI:** usa `visualize` para tablas, Mermaid o visualizaciones inline; para UI del producto inspecciona las convenciones existentes y valida desktop/mobile si aplica.
- **Shell o tareas operativas:** usa los comandos y entry points del repositorio, evita scripts destructivos y valida el resultado.
- **Objetivos iterativos, loops o autopilot:** solo usarlos cuando el usuario pide trabajo continuado o una condicion de parada automatica; fija checkpoints y stop conditions.
- **Reglas, skills, agentes o configuracion del editor:** usa la skill `agent-customization` y conserva las instrucciones del repositorio.

## Limites

No copies instrucciones especificas de Cursor que dependan de Canvas SDK, statusline, CLI config o APIs que no existan en VS Code. No sustituye `AGENTS.md`, `.cursor/rules` ni los tests de arquitectura. Cuando haya conflicto, prevalecen las reglas del repositorio y las instrucciones del agente.

## Comportamiento

La seleccion es interna: no pidas al usuario que diga que skill usar. Menciona solo el workflow aplicado si ayuda a explicar una decision o una validacion.
