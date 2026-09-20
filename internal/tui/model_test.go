package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/z-shell/zi-setup/internal/contract"
	"github.com/z-shell/zi-setup/internal/engine"
	"github.com/z-shell/zi-setup/internal/workflow"
)

func TestModelCompletesCompactKeyboardFlow(t *testing.T) {
	t.Parallel()
	fake := &modelEngine{}
	session := workflow.New(fake)
	model := New(context.Background(), session, Options{NoColor: true, ASCII: true})
	model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	discovery := model.Init()
	model.Update(discovery())
	if session.Stage != workflow.StageChoose {
		t.Fatalf("stage after discovery = %s", session.Stage)
	}
	if content := model.View().Content; !strings.Contains(content, "zi> setup") || !strings.Contains(content, "Choose a starting point") {
		t.Fatalf("compact choose view missing content:\n%s", content)
	}
	model.Update(tea.KeyPressMsg{Code: 'i'})
	if !session.SkipZshrc || !strings.Contains(model.View().Content, "leave .zshrc unchanged") {
		t.Fatalf("integration choice was not updated:\n%s", model.View().Content)
	}
	model.Update(tea.KeyPressMsg{Code: 'r'})
	model.refDraft = "v2.1.0"
	model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if session.Ref != "v2.1.0" {
		t.Fatalf("ref = %q", session.Ref)
	}
	model.cursor = 1
	planCommand := model.handleKey("enter")
	if planCommand == nil {
		t.Fatal("enter did not start planning")
	}
	model.Update(planCommand())
	if session.Stage != workflow.StageReview || session.Plan.ID == "" {
		t.Fatalf("review state = %s, plan = %#v", session.Stage, session.Plan)
	}
	if fake.ref != "v2.1.0" || !fake.skipZshrc {
		t.Fatalf("engine choices = ref %q, skip-zshrc %v", fake.ref, fake.skipZshrc)
	}
	model.handleKey("tab")
	if model.tab != 1 || !strings.Contains(model.View().Content, "Generated Zsh") {
		t.Fatalf("generated view was not selected:\n%s", model.View().Content)
	}
	model.handleKey("a")
	if !model.confirm || !strings.Contains(model.View().Content, session.Plan.ID) {
		t.Fatal("apply confirmation does not show exact plan id")
	}
	applyCommand := model.handleKey("y")
	if applyCommand == nil {
		t.Fatal("confirmation did not start apply")
	}
	if !model.busy || !model.applying || session.Stage != workflow.StageReview {
		t.Fatalf("in-flight apply state = busy %v, applying %v, stage %s", model.busy, model.applying, session.Stage)
	}
	if command := model.handleKey("ctrl+c"); command != nil || model.ctx.Err() != nil {
		t.Fatalf("ctrl+c interrupted in-flight apply: command %v, context %v", command, model.ctx.Err())
	}
	if content := model.footer(); !strings.Contains(content, "not interruptible") {
		t.Fatalf("busy footer does not disclose cancellation boundary: %q", content)
	}
	batch, ok := applyCommand().(tea.BatchMsg)
	if !ok || len(batch) != 2 {
		t.Fatalf("apply command returned %T", applyCommand())
	}
	model.Update(batch[0]())
	for eventCommand := batch[1]; eventCommand != nil; {
		_, eventCommand = model.Update(eventCommand())
	}
	if model.applying {
		t.Fatal("completed apply remained marked in flight")
	}
	if session.Stage != workflow.StageResult || !session.ReopenHasNoContentChanges() {
		t.Fatalf("result stage = %s, reopen unchanged = %v", session.Stage, session.ReopenHasNoContentChanges())
	}
	if fake.phases != "checkout,files" {
		t.Fatalf("phase order = %s", fake.phases)
	}
	if model.lastEvent == nil || model.lastEvent.Operation != "write-files" || model.lastEvent.Status != "succeeded" {
		t.Fatalf("last event = %#v", model.lastEvent)
	}
	if content := model.View().Content; !strings.Contains(content, "reopen: no content changes") {
		t.Fatalf("result view missing verification:\n%s", content)
	}
}

func TestRefEditorAcceptsOnlyPrintableRefText(t *testing.T) {
	t.Parallel()
	session := workflow.New(&modelEngine{})
	model := New(context.Background(), session, Options{NoColor: true})
	model.refEditing = true
	model.refDraft = "main"
	model.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	if model.refDraft != "main" {
		t.Fatalf("special key changed ref to %q", model.refDraft)
	}
	model.Update(tea.KeyPressMsg{Code: '/', Text: "/feature"})
	if model.refDraft != "main/feature" {
		t.Fatalf("printable text produced ref %q", model.refDraft)
	}
}

type modelEngine struct {
	phases    string
	ref       string
	skipZshrc bool
}

func (f *modelEngine) Configure(ref string, skipZshrc bool) error {
	f.ref = ref
	f.skipZshrc = skipZshrc
	return nil
}

func (f *modelEngine) Describe(context.Context) (contract.Describe, engine.Output, error) {
	return contract.Describe{
		Format: "zi-setup-describe-v1",
		Facts:  []contract.Fact{{ID: "git", Value: "available", Source: "observed", Confidence: "certain"}},
		Profiles: []contract.Profile{
			{ID: "loader", Title: "Zi only", Selectable: true},
			{ID: "annex", Title: "Zi with annexes", Selectable: true},
		},
	}, engine.Output{}, nil
}

func (f *modelEngine) Plan(context.Context, string) (contract.Plan, engine.Output, error) {
	return contract.Plan{
		Format: "zi-setup-plan-v1",
		ID:     strings.Repeat("b", 64),
		Meta:   contract.PlanMeta{Profile: "annex", Ref: "main", ConfigHome: "/fixture/config", CheckoutPath: "/fixture/data/bin"},
		Operations: []contract.Operation{
			{ID: "checkout-sync", Phase: "checkout", Kind: "clone", Summary: "Checkout"},
			{ID: "write-files", Phase: "files", Kind: "write-files", Summary: "Files"},
		},
		Targets: []contract.Target{{ID: "shell", Path: "/fixture/config/setup/shell.zsh", Content: []byte("fixture\n"), Changed: false}},
	}, engine.Output{}, nil
}

func (f *modelEngine) Apply(_ context.Context, phase string, onEvent func(contract.ApplyEvent)) (contract.Result, engine.Output, error) {
	if f.phases != "" {
		f.phases += ","
	}
	f.phases += phase
	operation := "checkout-sync"
	if phase == "files" {
		operation = "write-files"
	}
	if onEvent != nil {
		onEvent(contract.ApplyEvent{Format: "zi-setup-event-v1", Phase: phase, Operation: operation, Status: "started", Detail: "working"})
		onEvent(contract.ApplyEvent{Format: "zi-setup-event-v1", Phase: phase, Operation: operation, Status: "succeeded", Detail: "complete"})
	}
	return contract.Result{
		Format:     "zi-setup-result-v1",
		PlanID:     strings.Repeat("b", 64),
		Phase:      phase,
		Status:     "succeeded",
		Operations: []contract.ResultOperation{{ID: operation, Status: "succeeded"}},
	}, engine.Output{}, nil
}
