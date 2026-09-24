---
name: split-to-prs
description: Divide un conjunto de cambios en varios PRs pequeños y revisables. Usar cuando el usuario pida separar trabajo, organizar una rama o dividir un PR por responsabilidades.
---

# Split to PRs

Divide el trabajo por limites de propiedad, riesgo y responsabilidad, manteniendo cada cambio coherente y revisable.

## Flujo

1. Inspecciona cambios commiteados y no commiteados contra la rama base.
2. Identifica ownership, dependencias y slices naturales.
3. Propone una division breve con orden de integracion y pide aprobacion antes de crear ramas, commits o PRs.
4. Guarda un snapshot recuperable antes de mover trabajo sucio.
5. Para cada slice aprobado, crea una rama desde la base correcta, añade solo archivos o hunks nombrados, valida y publica.
6. Informa de PRs creados y de cualquier cambio que quede en la rama inicial.

## Reglas

- No uses `git add .` ni `git add -A`; añade rutas o hunks concretos.
- No descartes trabajo del usuario ni uses reset destructivo, clean, force-push o reescritura de historial.
- No mezcles slices independientes solo por comodidad.
- Mantén primero las capas fundacionales y despues sus consumidores cuando exista dependencia real.
- No abras PRs ni hagas push sin aprobacion explicita del plan de division.
