# GitHub Copilot in VS Code — match ParkXchange / Cursor habits

Cursor loads `AGENTS.md`, `.cursor/rules/*`, session context, and **skills**.
Copilot can do a large part of that via repo files — not a full clone of Cursor,
but enough to force interview → plan → TDD → verify when you fall back to VS Code.

## What is in the repo

| Path | Role |
| --- | --- |
| `.github/copilot-instructions.md` | Always-on behaviour + skill routing |
| `.github/instructions/*.instructions.md` | Go API / mobile path rules |
| `.github/skills/*/SKILL.md` | On-demand workflows (auto or `/skill-name`) |
| `.github/prompts/*.prompt.md` | Slash shortcuts: `/feature`, `/interview`, … |
| `AGENTS.md` / `PROGRESS.md` | Architecture + cross-session state |
| `.vscode/settings.json` | Enables instructions, `AGENTS.md`, prompts |

### Skills (Cursor-like)

| Skill | When |
| --- | --- |
| `brainstorming` | New feature — interview + design, **no code until OK** |
| `entrevistador-procesos` | Process/workflow still vague |
| `writing-plans` | Exhaustive plan under `docs/superpowers/plans/` |
| `test-driven-development` | Red → green per task |
| `critical-preflight` | Hunt P0/P1 before “done” |
| `verification-before-completion` | Real commands + evidence |
| `superpowers` | Multi-part / risky work |

### Prompts (type `/` in Copilot Chat)

| Prompt | Pipeline |
| --- | --- |
| `/feature` | Full: interview → plan → TDD → preflight → verify |
| `/interview` | Questions only |
| `/plan` | Plan file only |
| `/tdd` | Next task with TDD |
| `/verify` | Preflight + verification |

## One-time VS Code setup

1. Install **GitHub Copilot** + **GitHub Copilot Chat** (recent VS Code; Agent mode).
2. Open the **repo root** (folder with `AGENTS.md` + `.github/`), not only `apps/mobile`.
3. Sign in with Copilot access.
4. Use **Agent** mode for multi-file work.
5. Type `/` — you should see skills and prompts. Or `/skills` to configure discovery.
6. After a reply, check **References** for `copilot-instructions.md` / loaded skills.
7. Command Palette → **Chat: Open Customizations** (or “Customization Diagnostics”) to confirm skills/prompts are listed.

If skills do not appear: update VS Code + Copilot extensions; ensure you are not in a nested workspace that hides `.github/skills`.

## How to work when Cursor tokens run out

**Best default for a new feature:**

```text
/feature
<describe the idea in 2–5 lines>
```

Or manually:

```text
Read PROGRESS.md and AGENTS.md.
Use brainstorming (one question at a time). Do not code until I approve the design.
Then writing-plans, then TDD per task, then critical-preflight + verification-before-completion.
Task: <…>
```

Tips that matter more than settings:

- **@-attach** `AGENTS.md`, the plan, and the files you care about.
- Reject huge unrelated diffs; ask for the next plan task only.
- If it skips the interview and starts coding, stop it: “HARD-GATE: design first.”

## Personal Copilot instructions (optional)

GitHub / VS Code personal instructions, all repos:

- Prefer plan → smallest diff → verify with commands.
- For new features: interview one question at a time; no code before design OK.
- Never commit secrets; commit only when I ask.

Keep personal notes short; repo skills stay the source of process.

## Limits (be honest)

- Copilot may ignore a skill if the description does not match the chat — invoke `/brainstorming` or `/feature` explicitly.
- Context window and tool depth are weaker than Cursor; keep tasks small.
- Skills do not run scripts by themselves unless the agent chooses to; verification still needs the agent to run `task …`.

## Still better in Cursor when…

Long multi-phase work, EAS/Play Console coaching, or anything that needs many terminals and a held plan across hours. Use Copilot for bounded slices with `/feature` or `/tdd`.
