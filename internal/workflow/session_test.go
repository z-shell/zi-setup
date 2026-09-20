package workflow_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/z-shell/zi-setup/internal/contract"
	"github.com/z-shell/zi-setup/internal/engine"
	"github.com/z-shell/zi-setup/internal/workflow"
)

func TestLifecycleOrdersPhasesAndVerifiesReopen(t *testing.T) {
	t.Parallel()
	fake := &fakeEngine{}
	session := workflow.New(fake)
	ctx := context.Background()
	if err := session.DiscoverEnvironment(ctx); err != nil {
		t.Fatal(err)
	}
	if err := session.SelectProfile("annex"); err != nil {
		t.Fatal(err)
	}
	if err := session.BuildPlan(ctx); err != nil {
		t.Fatal(err)
	}
	if err := session.ApplyReviewedPlan(ctx); err != nil {
		t.Fatal(err)
	}
	if err := session.VerifyReopen(ctx); err != nil {
		t.Fatal(err)
	}
	if strings.Join(fake.applyCalls, ",") != "checkout,files" {
		t.Fatalf("phase order = %v", fake.applyCalls)
	}
	if session.Stage != workflow.StageResult || !session.ReopenHasNoContentChanges() {
		t.Fatalf("stage = %s, reopen unchanged = %v", session.Stage, session.ReopenHasNoContentChanges())
	}
}

func TestProfileAndReviewBoundaries(t *testing.T) {
	t.Parallel()
	session := workflow.New(&fakeEngine{})
	if err := session.SelectProfile("loader"); err == nil {
		t.Fatal("selection before discovery succeeded")
	}
	if err := session.DiscoverEnvironment(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := session.SelectProfile("zunit"); err == nil || !strings.Contains(err.Error(), "compatibility") {
		t.Fatalf("zunit selection error = %v", err)
	}
	if err := session.SelectProfile("unknown"); err == nil {
		t.Fatal("unknown selection succeeded")
	}
	if err := session.SelectProfile("loader"); err != nil {
		t.Fatal(err)
	}
	if err := session.BuildPlan(context.Background()); err != nil {
		t.Fatal(err)
	}
	session.BackToChoose()
	if session.Plan.ID != "" || session.Stage != workflow.StageChoose {
		t.Fatalf("back did not discard the plan")
	}
}

func TestPartialFailuresRemainVisible(t *testing.T) {
	t.Parallel()
	phaseErr := errors.New("phase failed")
	t.Run("checkout stops files", func(t *testing.T) {
		fake := &fakeEngine{failPhase: "checkout", phaseErr: phaseErr}
		session := preparedSession(t, fake)
		if err := session.ApplyReviewedPlan(context.Background()); !errors.Is(err, phaseErr) {
			t.Fatalf("error = %v", err)
		}
		if strings.Join(fake.applyCalls, ",") != "checkout" || session.Checkout == nil || session.Files != nil {
			t.Fatalf("partial state: calls=%v checkout=%#v files=%#v", fake.applyCalls, session.Checkout, session.Files)
		}
	})
	t.Run("files preserves checkout", func(t *testing.T) {
		fake := &fakeEngine{failPhase: "files", phaseErr: phaseErr}
		session := preparedSession(t, fake)
		if err := session.ApplyReviewedPlan(context.Background()); !errors.Is(err, phaseErr) {
			t.Fatalf("error = %v", err)
		}
		if strings.Join(fake.applyCalls, ",") != "checkout,files" || session.Checkout == nil || session.Files == nil {
			t.Fatalf("partial state: calls=%v checkout=%#v files=%#v", fake.applyCalls, session.Checkout, session.Files)
		}
	})
}

func preparedSession(t *testing.T, fake *fakeEngine) *workflow.Session {
	t.Helper()
	session := workflow.New(fake)
	ctx := context.Background()
	if err := session.DiscoverEnvironment(ctx); err != nil {
		t.Fatal(err)
	}
	if err := session.SelectProfile("loader"); err != nil {
		t.Fatal(err)
	}
	if err := session.BuildPlan(ctx); err != nil {
		t.Fatal(err)
	}
	return session
}

type fakeEngine struct {
	applyCalls []string
	failPhase  string
	phaseErr   error
	plans      int
}

func (f *fakeEngine) Describe(context.Context) (contract.Describe, engine.Output, error) {
	return contract.Describe{
		Format: "zi-setup-describe-v1",
		Profiles: []contract.Profile{
			{ID: "loader", Title: "Zi only", Selectable: true},
			{ID: "annex", Title: "Zi with annexes", Selectable: true},
			{ID: "zunit", Title: "Legacy zunit", Reason: "compatibility only"},
		},
	}, engine.Output{}, nil
}

func (f *fakeEngine) Plan(_ context.Context, profile string) (contract.Plan, engine.Output, error) {
	f.plans++
	return contract.Plan{
		Format:  "zi-setup-plan-v1",
		ID:      strings.Repeat("b", 64),
		Meta:    contract.PlanMeta{Profile: profile},
		Targets: []contract.Target{{ID: "init", Changed: f.plans == 1}},
	}, engine.Output{}, nil
}

func (f *fakeEngine) Apply(_ context.Context, phase string) (contract.Result, engine.Output, error) {
	f.applyCalls = append(f.applyCalls, phase)
	result := contract.Result{Format: "zi-setup-result-v1", PlanID: strings.Repeat("b", 64), Phase: phase, Status: "succeeded"}
	if phase == f.failPhase {
		result.Status = "failed"
		result.Error = &contract.ResultError{Code: "fixture-failed", Detail: "fixture phase failed"}
		return result, engine.Output{}, f.phaseErr
	}
	return result, engine.Output{}, nil
}
