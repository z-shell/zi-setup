package contract

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadApplyEvent(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	values := map[string]string{
		"format": "zi-setup-event-v1", "phase": "checkout", "operation": "checkout-sync",
		"status": "started", "detail": "fetching the requested ref",
	}
	for name, value := range values {
		if err := os.WriteFile(filepath.Join(root, name), []byte(value+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	event, err := ReadApplyEvent(root)
	if err != nil {
		t.Fatal(err)
	}
	if event.Operation != "checkout-sync" || event.Status != "started" {
		t.Fatalf("event = %#v", event)
	}
}

func TestReadApplyEventRejectsUnknownStatus(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	values := map[string]string{
		"format": "zi-setup-event-v1", "phase": "files", "operation": "write-files",
		"status": "almost-done", "detail": "fixture",
	}
	for name, value := range values {
		if err := os.WriteFile(filepath.Join(root, name), []byte(value+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := ReadApplyEvent(root); err == nil {
		t.Fatal("unknown event status was accepted")
	}
}
