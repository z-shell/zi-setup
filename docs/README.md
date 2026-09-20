# Zi Setup

Zi Setup is a terminal client for the versioned guided-setup engine in [`z-shell/src`](https://github.com/z-shell/src). It discovers a bounded set of facts, offers the engine-provided profiles, shows the exact generated plan, and applies the reviewed plan hash in separate checkout and file phases.

This pilot supports `loader` and `annex`. A discovered `zunit` profile is shown only as preserved compatibility content.

## Local pilot

Build the client:

```sh
go build -o bin/zi-setup ./cmd/zi-setup
```

Run it against a local checkout of the engine:

```sh
bin/zi-setup --engine /path/to/src/public/sh/setup.sh
```

Use `--plain` for a linear keyboard interaction. For a disposable test home, pass explicit paths so no real shell configuration is touched:

```sh
bin/zi-setup --plain \
  --engine /path/to/src/public/sh/setup.sh \
  --home /tmp/zi-setup-home \
  --config-home /tmp/zi-setup-config/zi \
  --zshrc /tmp/zi-setup-home/.zshrc
```

The setup engine remains the sole owner of discovery, planning, generated Zsh, precondition checks, file changes, checkout operations, and receipts.

## Verification

```sh
go test ./...
go test -race ./...
go vet ./...
```

See [architecture.md](architecture.md) for the pilot boundary and test strategy.

## Current limits

The pilot requires an explicit local `setup.sh` engine path. Release packaging, a verified engine-bundle bootstrap, streaming apply events, and additional capability choices remain separate delivery work.
