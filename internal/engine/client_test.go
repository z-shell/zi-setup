package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspacePassesExactPlanHashToBothPhases(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	enginePath := filepath.Join(home, "setup.sh")
	if err := os.WriteFile(enginePath, []byte("# fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	shellPath := filepath.Join(home, "fake-shell")
	if err := os.WriteFile(shellPath, []byte(fakeShell), 0o700); err != nil {
		t.Fatal(err)
	}
	workspace, err := (Client{EnginePath: enginePath, ShellPath: shellPath, TempParent: home}).NewWorkspace(Inputs{
		Home:       home,
		ConfigHome: filepath.Join(home, "config home"),
		Zshrc:      filepath.Join(home, "dot files", ".zshrc"),
		ZiHome:     filepath.Join(home, "data home"),
		Ref:        "feature/ref",
	})
	if err != nil {
		t.Fatal(err)
	}
	root := workspace.Root()
	if info, err := os.Stat(root); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("artifact root mode = %v, err = %v", info.Mode().Perm(), err)
	}
	describe, _, err := workspace.Describe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(describe.Profiles) != 2 || !describe.Profiles[1].Selectable {
		t.Fatalf("profiles = %#v", describe.Profiles)
	}
	plan, _, err := workspace.Plan(context.Background(), "annex")
	if err != nil {
		t.Fatal(err)
	}
	if plan.ID != strings.Repeat("a", 64) || plan.Meta.Profile != "annex" {
		t.Fatalf("plan = %#v", plan)
	}
	for _, phase := range []string{"checkout", "files"} {
		result, _, err := workspace.Apply(context.Background(), phase)
		if err != nil {
			t.Fatalf("apply %s: %v", phase, err)
		}
		if result.PlanID != plan.ID || result.Status != "succeeded" {
			t.Fatalf("%s result = %#v", phase, result)
		}
	}
	log, err := os.ReadFile(filepath.Join(home, "calls"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(log)
	if strings.Count(text, "--expect="+plan.ID) != 2 {
		t.Fatalf("exact plan hash was not passed twice:\n%s", text)
	}
	for _, value := range []string{"--config-home=" + filepath.Join(home, "config home"), "--zshrc=" + filepath.Join(home, "dot files", ".zshrc"), "--ref=feature/ref"} {
		if !strings.Contains(text, value) {
			t.Errorf("invocation log missing %q:\n%s", value, text)
		}
	}
	if err := workspace.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("artifact root remains after Close: %v", err)
	}
}

func TestWorkspaceRejectsMismatchedResultPhase(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	enginePath := filepath.Join(home, "setup.sh")
	if err := os.WriteFile(enginePath, []byte("# fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	shellPath := filepath.Join(home, "fake-shell")
	fixture := strings.Replace(fakeShell, "printf '%s\\n' \"$phase\" >\"$result/phase\"", "printf '%s\\n' files >\"$result/phase\"", 1)
	fixture = strings.Replace(fixture, "operation=checkout-sync\n  [ \"$phase\" = checkout ] || operation=write-files", "operation=write-files", 1)
	if err := os.WriteFile(shellPath, []byte(fixture), 0o700); err != nil {
		t.Fatal(err)
	}
	workspace, err := (Client{EnginePath: enginePath, ShellPath: shellPath, TempParent: home}).NewWorkspace(Inputs{Home: home})
	if err != nil {
		t.Fatal(err)
	}
	defer workspace.Close()
	if _, _, err := workspace.Plan(context.Background(), "loader"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := workspace.Apply(context.Background(), "checkout"); err == nil || !strings.Contains(err.Error(), `checkout apply returned a "files" result`) {
		t.Fatalf("phase mismatch error = %v", err)
	}
}

const fakeShell = `#!/bin/sh
set -eu
engine=$1
shift
command=$1
shift
(
  printf 'command=%s\n' "$command"
  while [ "$#" -gt 0 ]; do
    key=$1
    case "$key" in
      --skip-zshrc) printf '%s\n' "$key"; shift ;;
      *) printf '%s=%s\n' "$key" "$2"; shift 2 ;;
    esac
  done
  printf '%s\n' '---'
) >>"$HOME/calls"

find_arg() {
  wanted=$1
  shift
  while [ "$#" -gt 0 ]; do
    if [ "$1" = "$wanted" ]; then printf '%s\n' "$2"; return; fi
    case "$1" in --skip-zshrc) shift ;; *) shift 2 ;; esac
  done
  return 1
}

case "$command" in
describe)
  output=$(find_arg --output "$@")
  mkdir -p "$output/facts" "$output/profiles"
  printf '%s\n' zi-setup-describe-v1 >"$output/format"
  printf '%s\n' config-home zi-home checkout-path zi-home-state zshrc-path zshrc-state git zsh tty existing-profile >"$output/facts/order"
  for id in config-home zi-home checkout-path zi-home-state zshrc-path zshrc-state git zsh tty existing-profile; do
    mkdir -p "$output/facts/$id"
    case "$id" in
      zi-home-state) value=selected ;;
      zshrc-state) value=missing ;;
      git|zsh) value=available ;;
      tty) value=no ;;
      existing-profile) value=none ;;
      *) value=/fixture ;;
    esac
    printf '%s\n' "$value" >"$output/facts/$id/value"
    printf '%s\n' observed >"$output/facts/$id/source"
    printf '%s\n' certain >"$output/facts/$id/confidence"
  done
  printf '%s\n' loader annex >"$output/profiles/order"
  for id in loader annex; do
    mkdir -p "$output/profiles/$id"
    printf '%s\n' yes >"$output/profiles/$id/selectable"
    printf '%s\n' Ready >"$output/profiles/$id/reason"
    printf '%s\n' "$id" >"$output/profiles/$id/title"
  done
  ;;
plan)
  plan=$(find_arg --plan "$@")
  profile=$(find_arg --profile "$@")
  mkdir -p "$plan/checkout" "$plan/targets/init" "$plan/operations/checkout-sync" "$plan/operations/write-files" "$plan/warnings"
  printf '%064s\n' '' | tr ' ' a >"$plan/plan.id"
  cat >"$plan/plan.meta" <<EOF
format=zi-setup-plan-v1
profile=$profile
ref=main
config_home=/fixture/config
checkout_path=/fixture/data/bin
receipt_path=/fixture/config/setup/receipt
skip_zshrc=0
EOF
  printf '%s\n' missing >"$plan/checkout/kind"
  printf '%s\n' missing >"$plan/checkout/head"
  printf '%s\n' missing >"$plan/checkout/current-ref"
  printf '%s\n' missing >"$plan/checkout/origin"
  printf '%s\n' main >"$plan/checkout/requested-ref"
  printf '%s\n' init >"$plan/targets/order"
  printf '%s\n' /fixture/config/init.zsh >"$plan/targets/init/path"
  printf '%s\n' missing >"$plan/targets/init/kind"
  printf '%s\n' missing >"$plan/targets/init/expected"
  printf '%s\n' 755 >"$plan/targets/init/mode"
  printf '%s\n' fixture >"$plan/targets/init/content"
  printf '%s\n' checkout-sync write-files >"$plan/operations/order"
  printf '%s\n' checkout >"$plan/operations/checkout-sync/phase"
  printf '%s\n' clone >"$plan/operations/checkout-sync/kind"
  printf '%s\n' Checkout >"$plan/operations/checkout-sync/summary"
  printf '%s\n' no >"$plan/operations/checkout-sync/interruptible"
  printf '%s\n' files >"$plan/operations/write-files/phase"
  printf '%s\n' write-files >"$plan/operations/write-files/kind"
  printf '%s\n' Files >"$plan/operations/write-files/summary"
  printf '%s\n' no >"$plan/operations/write-files/interruptible"
  : >"$plan/warnings/order"
  ;;
apply)
  plan=$(find_arg --plan "$@")
  phase=$(find_arg --phase "$@")
  result=$(find_arg --result "$@")
  plan_id=$(cat "$plan/plan.id")
  operation=checkout-sync
  [ "$phase" = checkout ] || operation=write-files
  mkdir -p "$result/operations/$operation"
  printf '%s\n' zi-setup-result-v1 >"$result/format"
  printf '%s\n' "$plan_id" >"$result/plan.id"
  printf '%s\n' "$phase" >"$result/phase"
  printf '%s\n' succeeded >"$result/status"
  printf '%s\n' "$operation" >"$result/operations/order"
  printf '%s\n' succeeded >"$result/operations/$operation/status"
  printf '%s\n' complete >"$result/operations/$operation/detail"
  if [ "$phase" = files ]; then
    mkdir -p "$result/receipt"
    printf '%s\n' /fixture/config/setup/receipt >"$result/receipt/path"
  fi
  ;;
esac
`
