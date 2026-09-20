# Project guidelines - zi-setup

This project follows the organization-wide [Z-Shell Organization Guidelines](https://github.com/z-shell/.github/blob/main/AGENTS.md).

## Scope

`zi-setup` is a standalone Go client for the versioned setup engine owned by `z-shell/src`. It presents engine artifacts and orchestrates engine commands. It must not resolve Zi paths, generate Zsh, edit startup files, or reinterpret human-readable engine output.

## Development

- Use Go 1.25 or newer and the versioned Charm v2 module paths.
- Keep TUI, plain, and headless modes on the same contract reader and workflow.
- Pass engine arguments as arrays. Never evaluate artifact content or parse human-readable stdout or stderr for decisions.
- Treat display text and paths as untrusted terminal content.
- Test with `go test ./...` and run `go vet ./...` before review.
- Branches use the organization `feature-<id>`, `bug-<id>`, or `hotfix-<id>` shape. Pull requests target `main`.
