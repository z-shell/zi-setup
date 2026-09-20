package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/z-shell/zi-setup/internal/contract"
)

func TestBundledEngineDescribesAndPlansDisposableHome(t *testing.T) {
	home := t.TempDir()
	fakeBin := t.TempDir()
	if err := os.WriteFile(filepath.Join(fakeBin, "zsh"), []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	workspace, err := (Client{TempParent: t.TempDir()}).NewWorkspace(Inputs{
		Home:       home,
		ConfigHome: filepath.Join(home, ".config", "zi"),
		ZiHome:     filepath.Join(home, ".local", "share", "zi"),
		Zshrc:      filepath.Join(home, ".zshrc"),
		SkipZshrc:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer workspace.Close()
	describe, _, err := workspace.Describe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if describe.Format != "zi-setup-describe-v1" {
		t.Fatalf("describe format = %q", describe.Format)
	}
	plan, _, err := workspace.Plan(context.Background(), "loader")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Format != "zi-setup-plan-v1" || plan.Meta.Ref != "main" || !plan.Meta.SkipZshrc {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestBundledEngineAppliesBothPhasesWithEvents(t *testing.T) {
	home := t.TempDir()
	fakeBin := filepath.Join(home, "bin")
	if err := os.Mkdir(fakeBin, 0o700); err != nil {
		t.Fatal(err)
	}
	fakeGit := `#!/bin/sh
set -eu
case "$1" in
check-ref-format)
  [ "$2" = --branch ]
  printf '%s\n' "$3"
  ;;
clone)
  destination=
  for argument do destination=$argument; done
  mkdir -p "$destination/.git"
  printf '%s\n' '# fixture' >"$destination/zi.zsh"
  ;;
-C)
  shift 2
  case "$1 $2 ${3:-}" in
  'remote get-url origin') printf '%s\n' https://github.com/z-shell/zi.git ;;
  'rev-parse HEAD ') printf '%040d\n' 1 ;;
  'symbolic-ref --quiet --short') printf '%s\n' main ;;
  *) exit 64 ;;
  esac
  ;;
*) exit 64 ;;
esac
`
	if err := os.WriteFile(filepath.Join(fakeBin, "git"), []byte(fakeGit), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

	workspace, err := (Client{TempParent: home}).NewWorkspace(Inputs{
		Home:       home,
		ConfigHome: filepath.Join(home, ".config", "zi"),
		ZiHome:     filepath.Join(home, ".local", "share", "zi"),
		Zshrc:      filepath.Join(home, ".zshrc"),
		SkipZshrc:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer workspace.Close()
	if _, _, err := workspace.Plan(context.Background(), "loader"); err != nil {
		t.Fatal(err)
	}

	var events []contract.ApplyEvent
	for _, phase := range []string{"checkout", "files"} {
		result, _, err := workspace.Apply(context.Background(), phase, func(event contract.ApplyEvent) {
			events = append(events, event)
		})
		if err != nil {
			t.Fatalf("apply %s: %v", phase, err)
		}
		if result.Status != "succeeded" || result.Phase != phase {
			t.Fatalf("%s result = %#v", phase, result)
		}
	}
	if len(events) != 4 {
		t.Fatalf("events = %#v", events)
	}
	for offset := 0; offset < len(events); offset += 2 {
		if events[offset].Sequence != 1 || events[offset].Status != "started" || events[offset+1].Sequence != 2 || events[offset+1].Status != "succeeded" {
			t.Fatalf("phase events = %#v", events[offset:offset+2])
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "zi", "init.zsh")); err != nil {
		t.Fatalf("installed init.zsh: %v", err)
	}
}

func TestBundledEngineIsPinnedAndExtractedPrivately(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	var inputs Inputs
	path, err := extractBundledEngine(root, &inputs)
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range []string{path, inputs.Init, inputs.Profiles, inputs.Checksum} {
		info, err := os.Stat(candidate)
		if err != nil {
			t.Fatalf("stat %s: %v", candidate, err)
		}
		if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
			t.Fatalf("%s mode = %s", candidate, info.Mode())
		}
		if filepath.Clean(candidate) != candidate {
			t.Fatalf("unclean extracted path %q", candidate)
		}
	}
}

func TestBundledEngineVerificationRejectsTampering(t *testing.T) {
	t.Parallel()
	fixture := fstest.MapFS{}
	for name := range bundledHashes {
		fixture["bundled/"+name] = &fstest.MapFile{Data: []byte("tampered\n")}
	}
	if err := verifyBundle(fixture); err == nil {
		t.Fatal("tampered bundle passed verification")
	}
}
