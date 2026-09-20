# Pilot architecture

## Boundary

Zi Setup is a client of `public/sh/setup.sh`. The engine owns resolved paths, profile availability, generated Zsh, checkout operations, preconditions, application, and receipts. The client owns interaction, presentation, artifact validation, control-sequence sanitization, and process orchestration.

The client uses only these versioned contracts:

- `zi-setup-describe-v1`
- `zi-setup-plan-v1`
- `zi-setup-result-v1`

Human-readable engine output is captured for an optional sanitized details view. It never controls a decision.

## Packages

- `internal/contract` reads and validates fixed artifact paths.
- `internal/engine` invokes `setup.sh` with argument arrays and private artifact directories.
- `internal/workflow` owns the shared discover, plan, and phased-apply state.
- `internal/plain` provides linear interaction over the shared workflow.
- `internal/tui` renders the same workflow with Bubble Tea v2.

## Approval and application

Review stores the exact `plan.id`. Both checkout and files phases pass it to the engine through `--expect`. A changed plan is rejected by the engine. The phases publish separate result artifacts so partial completion stays visible.

Going backward discards the plan and creates a new one. No plan artifact is edited in place.

## Safety

- Every engine argument is a distinct process argument.
- Artifact versions, IDs, order files, and restricted tokens are validated.
- Display text has terminal control bytes escaped before rendering.
- Temporary roots use private OS-created directories.
- Plain and TUI modes use the same workflow object and engine arguments.
- Tests use fake engines or disposable homes. Development never targets the maintainer's real shell configuration.
