package presentation

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/z-shell/zi-setup/internal/contract"
)

func TestSafeTextEscapesTerminalControls(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"plain":                 "plain",
		"\x1b[31mred\x1b[0m":    "^[[31mred^[[0m",
		"a\tb\n":                "a    b\n",
		"\x00\x07\r\x7f":        "\\u0000\\u0007\\u000d\\u007f",
		string([]byte{0xff, 1}): "\\xff\\u0001",
	}
	for input, expected := range tests {
		if actual := SafeText(input); actual != expected {
			t.Errorf("SafeText(%q) = %q, want %q", input, actual, expected)
		}
	}
}

func TestPlanViewsSanitizeDisplayText(t *testing.T) {
	t.Parallel()
	plan := contract.Plan{
		ID:         strings.Repeat("a", 64),
		Meta:       contract.PlanMeta{Profile: "loader", Ref: "main\x1b[31m", ConfigHome: "/config", CheckoutPath: "/checkout"},
		Operations: []contract.Operation{{Phase: "files\x1b[2J", Kind: "write-files\x1b[2J", Summary: "write\x1b[2J"}},
		Warnings:   []contract.Warning{{Severity: "warning", Summary: "warn\x07", Remediation: "fix\r"}},
		Targets:    []contract.Target{{ID: "entry", Path: "/config/setup.zsh", Content: []byte("print '\x1b[2J'\n"), Changed: true}},
	}
	for name, output := range map[string]string{"plan": PlanText(plan), "generated": GeneratedText(plan), "experience": ExperiencePreview(plan.Meta.Profile)} {
		if strings.Contains(output, "\x1b") || strings.Contains(output, "\x07") || strings.Contains(output, "\r") {
			t.Errorf("%s contains a raw terminal control: %q", name, output)
		}
	}
}

func TestResultTextSanitizesErrorCode(t *testing.T) {
	t.Parallel()
	result := contract.Result{Status: "failed", Error: &contract.ResultError{Code: "bad\x1b[2J", Detail: "detail"}}
	if output := ResultText(&result, nil, nil); strings.Contains(output, "\x1b") {
		t.Fatalf("result contains a raw terminal control: %q", output)
	}
}

func TestDiffTextUsesReviewedPrecondition(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	path := filepath.Join(root, "shell.zsh")
	current := []byte("one\ntwo\n")
	if err := os.WriteFile(path, current, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(current)
	target := contract.Target{ID: "shell", Path: path, Expected: hex.EncodeToString(digest[:]), Content: []byte("one\nthree\n"), Changed: true}
	diff := DiffText(contract.Plan{Targets: []contract.Target{target}})
	for _, expected := range []string{" one\n", "-two\n", "+three\n"} {
		if !strings.Contains(diff, expected) {
			t.Errorf("diff missing %q:\n%s", expected, diff)
		}
	}
	if err := os.WriteFile(path, []byte("drift\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if drift := DiffText(contract.Plan{Targets: []contract.Target{target}}); !strings.Contains(drift, "target changed after planning") {
		t.Fatalf("drift was not shown: %s", drift)
	}
	if unchanged := DiffText(contract.Plan{Targets: []contract.Target{{Changed: false}}}); unchanged != "No content changes.\n" {
		t.Fatalf("unchanged = %q", unchanged)
	}
}
