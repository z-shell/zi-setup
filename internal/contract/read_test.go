package contract

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadVersionedArtifacts(t *testing.T) {
	t.Parallel()
	t.Run("describe", func(t *testing.T) {
		describe, err := ReadDescribe(describeFixture(t))
		if err != nil {
			t.Fatal(err)
		}
		if len(describe.Facts) != 10 || len(describe.Profiles) != 2 || !describe.Profiles[1].Selectable {
			t.Fatalf("describe = %#v", describe)
		}
	})
	t.Run("plan", func(t *testing.T) {
		plan, err := ReadPlan(planFixture(t))
		if err != nil {
			t.Fatal(err)
		}
		if plan.Meta.Profile != "annex" || plan.Checkout.Kind != "missing" || len(plan.Targets) != 1 || !plan.Targets[0].Changed {
			t.Fatalf("plan = %#v", plan)
		}
		if len(plan.Operations) != 2 || len(plan.Warnings) != 1 {
			t.Fatalf("plan metadata = %#v", plan)
		}
	})
	t.Run("result", func(t *testing.T) {
		root := t.TempDir()
		writeFields(t, root, map[string]string{
			"format":                        "zi-setup-result-v1\n",
			"plan.id":                       strings.Repeat("a", 64) + "\n",
			"phase":                         "files\n",
			"status":                        "failed\n",
			"operations/order":              "write-files\n",
			"operations/write-files/status": "failed\n",
			"operations/write-files/detail": "write stopped\n",
			"error/code":                    "target-drift\n",
			"error/operation":               "write-files\n",
			"error/detail":                  "target changed\n",
		})
		result, err := ReadResult(root)
		if err != nil {
			t.Fatal(err)
		}
		if result.Status != "failed" || result.Error == nil || result.Error.Code != "target-drift" {
			t.Fatalf("result = %#v", result)
		}
	})
}

func TestRejectsUnknownOrHostileArtifacts(t *testing.T) {
	t.Parallel()
	t.Run("unknown format", func(t *testing.T) {
		root := describeFixture(t)
		writeFields(t, root, map[string]string{"format": "zi-setup-describe-v2\n"})
		if _, err := ReadDescribe(root); err == nil || !strings.Contains(err.Error(), "unsupported describe format") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("hostile order id", func(t *testing.T) {
		root := describeFixture(t)
		writeFields(t, root, map[string]string{"profiles/order": "loader\n../escape\n"})
		if _, err := ReadDescribe(root); err == nil || !strings.Contains(err.Error(), "invalid id") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("restricted token", func(t *testing.T) {
		root := describeFixture(t)
		writeFields(t, root, map[string]string{"facts/git/source": "observed\nunknown\n"})
		if _, err := ReadDescribe(root); err == nil || !strings.Contains(err.Error(), "single-line") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("duplicate plan metadata", func(t *testing.T) {
		root := planFixture(t)
		file := filepath.Join(root, "plan.meta")
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, append(data, []byte("profile=loader\n")...), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadPlan(root); err == nil || !strings.Contains(err.Error(), "duplicate key") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("relative target path", func(t *testing.T) {
		root := planFixture(t)
		writeFields(t, root, map[string]string{"targets/init/path": "../../etc/passwd\n"})
		if _, err := ReadPlan(root); err == nil || !strings.Contains(err.Error(), "clean and absolute") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("result plan id", func(t *testing.T) {
		root := resultFixture(t)
		writeFields(t, root, map[string]string{"plan.id": "not-a-hash\n"})
		if _, err := ReadResult(root); err == nil || !strings.Contains(err.Error(), "SHA-256") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("symlink root", func(t *testing.T) {
		realRoot := describeFixture(t)
		link := filepath.Join(t.TempDir(), "artifact")
		if err := os.Symlink(realRoot, link); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadDescribe(link); err == nil || !strings.Contains(err.Error(), "must be a directory") {
			t.Fatalf("error = %v", err)
		}
	})
}

func resultFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFields(t, root, map[string]string{
		"format":                          "zi-setup-result-v1\n",
		"plan.id":                         strings.Repeat("a", 64) + "\n",
		"phase":                           "checkout\n",
		"status":                          "succeeded\n",
		"operations/order":                "checkout-sync\n",
		"operations/checkout-sync/status": "succeeded\n",
		"operations/checkout-sync/detail": "complete\n",
	})
	return root
}

func describeFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	facts := []string{"config-home", "zi-home", "checkout-path", "zi-home-state", "zshrc-path", "zshrc-state", "git", "zsh", "tty", "existing-profile"}
	fields := map[string]string{
		"format":         "zi-setup-describe-v1\n",
		"facts/order":    strings.Join(facts, "\n") + "\n",
		"profiles/order": "loader\nannex\n",
	}
	for _, id := range facts {
		value := "/fixture"
		switch id {
		case "zi-home-state":
			value = "selected"
		case "zshrc-state":
			value = "missing"
		case "git", "zsh":
			value = "available"
		case "tty":
			value = "no"
		case "existing-profile":
			value = "none"
		}
		fields["facts/"+id+"/value"] = value + "\n"
		fields["facts/"+id+"/source"] = "observed\n"
		fields["facts/"+id+"/confidence"] = "certain\n"
	}
	for _, id := range []string{"loader", "annex"} {
		fields["profiles/"+id+"/selectable"] = "yes\n"
		fields["profiles/"+id+"/reason"] = "Ready\n"
		fields["profiles/"+id+"/title"] = id + "\n"
	}
	writeFields(t, root, fields)
	return root
}

func planFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFields(t, root, map[string]string{
		"plan.id":                                   strings.Repeat("a", 64) + "\n",
		"plan.meta":                                 "format=zi-setup-plan-v1\nprofile=annex\nref=main\nconfig_home=/fixture/config\ncheckout_path=/fixture/data/bin\nreceipt_path=/fixture/config/setup/receipt\nskip_zshrc=0\n",
		"checkout/kind":                             "missing\n",
		"checkout/head":                             "missing\n",
		"checkout/current-ref":                      "missing\n",
		"checkout/origin":                           "missing\n",
		"checkout/requested-ref":                    "main\n",
		"targets/order":                             "init\n",
		"targets/init/path":                         "/fixture/config/init.zsh\n",
		"targets/init/kind":                         "missing\n",
		"targets/init/expected":                     "missing\n",
		"targets/init/mode":                         "755\n",
		"targets/init/content":                      "fixture\n",
		"operations/order":                          "checkout-sync\nwrite-files\n",
		"operations/checkout-sync/phase":            "checkout\n",
		"operations/checkout-sync/kind":             "clone\n",
		"operations/checkout-sync/summary":          "Clone Zi\n",
		"operations/checkout-sync/interruptible":    "no\n",
		"operations/write-files/phase":              "files\n",
		"operations/write-files/kind":               "write-files\n",
		"operations/write-files/summary":            "Write files\n",
		"operations/write-files/interruptible":      "no\n",
		"warnings/order":                            "deferred-first-start\n",
		"warnings/deferred-first-start/severity":    "info\n",
		"warnings/deferred-first-start/summary":     "Deferred\n",
		"warnings/deferred-first-start/remediation": "Start Zsh later\n",
	})
	return root
}

func writeFields(t *testing.T, root string, fields map[string]string) {
	t.Helper()
	for name, value := range fields {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}
