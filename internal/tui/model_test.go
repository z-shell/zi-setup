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
	model.cursor = 1
	planCommand := model.handleKey("enter")
	if planCommand == nil {
		t.Fatal("enter did not start planning")
	}
	model.Update(planCommand())
	if session.Stage != workflow.StageReview || session.Plan.ID == "" {
		t.Fatalf("review state = %s, plan = %#v", session.Stage, session.Plan)
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
	model.Update(applyCommand())
	if session.Stage != workflow.StageResult || !session.ReopenHasNoContentChanges() {
		t.Fatalf("result stage = %s, reopen unchanged = %v", session.Stage, session.ReopenHasNoContentChanges())
	}
	if fake.phases != "checkout,files" {
		t.Fatalf("phase order = %s", fake.phases)
	}
	if content := model.View().Content; !strings.Contains(content, "reopen: no content changes") {
		t.Fatalf("result view missing verification:\n%s", content)
	}
}

func TestApplyIgnoresInterruptKey(t *testing.T) {
	t.Parallel()
	session := workflow.New(&modelEngine{})
	session.Stage = workflow.StageApply
	model := New(context.Background(), session, Options{NoColor: true})
	model.busy = true
	model.busyLabel = "Applying checkout, then files"
	if command := model.handleKey("ctrl+c"); command != nil {
		t.Fatal("ctrl+c interrupted a non-interruptible apply")
	}
	if content := model.footer(); !strings.Contains(content, "not interruptible") {
		t.Fatalf("busy footer does not disclose cancellation boundary: %q", content)
	}
}

type modelEngine struct {
	phases string
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

func (f *modelEngine) Apply(_ context.Context, phase string) (contract.Result, engine.Output, error) {
	if f.phases != "" {
		f.phases += ","
	}
	f.phases += phase
	operation := "checkout-sync"
	if phase == "files" {
		operation = "write-files"
	}
	return contract.Result{
		Format:     "zi-setup-result-v1",
		PlanID:     strings.Repeat("b", 64),
		Phase:      phase,
		Status:     "succeeded",
		Operations: []contract.ResultOperation{{ID: operation, Status: "succeeded"}},
	}, engine.Output{}, nil
}
