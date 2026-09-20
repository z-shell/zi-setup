# Zi Setup

Zi Setup is a terminal client for the versioned guided-setup engine in [`z-shell/src`](https://github.com/z-shell/src). It discovers a bounded set of facts, offers the engine-provided profiles, shows the exact generated plan, and applies the reviewed plan hash in separate checkout and file phases.

This pilot supports `loader` and `annex`. A discovered `zunit` profile is shown only as preserved compatibility content.

## Install

Versioned releases provide archives for Linux, macOS, and Windows on amd64 and arm64. Each release includes `SHA256SUMS` and GitHub artifact attestations. Windows execution requires a POSIX `sh` on `PATH`, such as Git Bash or MSYS2.

Download the archive for your platform from [GitHub Releases](https://github.com/z-shell/zi-setup/releases), verify it against `SHA256SUMS`, then place `zi-setup` on your `PATH`.

The binary includes a checksum-verified snapshot of the authoritative setup engine, so the normal invocation is:

```sh
zi-setup
```

Use `--version` to print both the client version and bundled `z-shell/src` revision.

## Build from source

Build the client:

```sh
go build -o bin/zi-setup ./cmd/zi-setup
```

Run with the bundled engine:

```sh
bin/zi-setup
```

Override it with a local engine checkout when developing the engine contract:

```sh
bin/zi-setup --engine /path/to/src/public/sh/setup.sh
```

Use `--plain` for a linear keyboard interaction. For a disposable test home, pass explicit paths so no real shell configuration is touched:

```sh
bin/zi-setup --plain \
  --home /tmp/zi-setup-home \
  --config-home /tmp/zi-setup-config/zi \
  --zshrc /tmp/zi-setup-home/.zshrc
```

The setup engine remains the sole owner of discovery, planning, generated Zsh, precondition checks, file changes, checkout operations, and receipts.

The terminal UI offers only engine-backed choices: the `loader` or `annex` profile, the Zi Git ref, and whether setup should update `.zshrc`. The discovered `zunit` profile remains visible only as preserved compatibility content. Plain and headless modes expose the same choices as flags.

An external engine defaults to phase-level progress for backward compatibility. Pass `--engine-events` only when that engine implements `zi-setup-event-v1`.

## Verification

```sh
go test ./...
go test -race ./...
go vet ./...
```

See [architecture.md](architecture.md) for the pilot boundary and test strategy.

## Engine provenance

The embedded files are copied byte-for-byte from a documented `z-shell/src` commit. Their SHA-256 values are compiled into the client and verified before extraction. `scripts/sync-engine.sh /path/to/src FULL_COMMIT_SHA` reads every asset from that exact commit, even when the source checkout has local changes. Maintainers then update the pinned revision and reported hashes in `internal/engine/bundle.go` and run the full checks.
