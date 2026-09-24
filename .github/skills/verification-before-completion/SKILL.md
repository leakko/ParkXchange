---
name: verification-before-completion
description: >-
  Use before claiming done, fixed, or passing, and before commits/PRs.
  Run real verification commands, read output, then claim with evidence.
  Never assert success from memory or partial runs.
---

# Verification before completion

## Ley

```
NO COMPLETION CLAIMS WITHOUT FRESH VERIFICATION EVIDENCE
```

## Gate

Antes de decir “listo”, “pasa”, “fixed”:

1. **IDENTIFY** el comando que prueba la afirmación.
2. **RUN** el comando completo (fresco).
3. **READ** salida + exit code.
4. **VERIFY** que confirma la afirmación.
5. Solo entonces afirma, citando evidencia breve.

## Comandos habituales aquí

- API / capas: `task db:up` luego `task api:test`
- Suite amplia: `task test`
- Mobile tipos: desde `apps/mobile`, el check TypeScript del proyecto
- Salud prod: `curl` al health de la API si el cambio es deploy-related

## También revisar (sin sustituir tests)

- Imports / capas que rompan `arch_test`
- i18n es+en si tocaste strings
- Permisos ubicación: no pedir Always en cold start
- Secrets en el diff

Si no pudiste ejecutar algo, dilo explícitamente; no inventes el resultado.
