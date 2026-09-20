package plain

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/z-shell/zi-setup/internal/contract"
	"github.com/z-shell/zi-setup/internal/engine"
	"github.com/z-shell/zi-setup/internal/workflow"
)

func TestRunRequiresExactPlanApproval(t *testing.T) {
	t.Parallel()
	planID := strings.Repeat("a", 64)
	tests := []struct {
		name       string
		input      string
		yes        bool
		wantErr    bool
		wantPhases string
	}{
		{name: "reject wrong hash", input: "apply wrong\n", wantErr: true},
		{name: "accept exact hash", input: "apply " + planID + "\n", wantPhases: "checkout,files"},
		{name: "explicit yes", yes: true, wantPhases: "checkout,files"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			fake := &plainEngine{planID: planID}
			var output bytes.Buffer
			err := Run(context.Background(), workflow.New(fake), Options{
				Profile: "loader",
				Apply:   true,
				Yes:     test.yes,
				Input:   strings.NewReader(test.input),
				Output:  &output,
			})
			if (err != nil) != test.wantErr {
				t.Fatalf("Run() error = %v, wantErr %v", err, test.wantErr)
			}
			if got := strings.Join(fake.phases, ","); got != test.wantPhases {
				t.Fatalf("apply phases = %q, want %q", got, test.wantPhases)
			}
			if !test.yes && !strings.Contains(output.String(), "Type apply "+planID) {
				t.Fatalf("output does not bind approval to plan id:\n%s", output.String())
			}
			if !test.wantErr && !strings.Contains(output.String(), "[checkout] checkout-sync started: synchronizing checkout") {
				t.Fatalf("output does not include structured progress:\n%s", output.String())
			}
		})
	}
}

type plainEngine struct {
	planID string
	phases []string
}

func (f *plainEngine) Configure(string, bool) error { return nil }

func (f *plainEngine) Describe(context.Context) (contract.Describe, engine.Output, error) {
	return contract.Describe{
		Format:   "zi-setup-describe-v1",
		Profiles: []contract.Profile{{ID: "loader", Title: "Zi only", Selectable: true}},
	}, engine.Output{}, nil
}

func (f *plainEngine) Plan(context.Context, string) (contract.Plan, engine.Output, error) {
	return contract.Plan{
		Format: "zi-setup-plan-v1",
		ID:     f.planID,
		Meta:   contract.PlanMeta{Profile: "loader"},
	}, engine.Output{}, nil
}

func (f *plainEngine) Apply(_ context.Context, phase string, onEvent func(contract.ApplyEvent)) (contract.Result, engine.Output, error) {
	f.phases = append(f.phases, phase)
	operation := "checkout-sync"
	if phase == "files" {
		operation = "write-files"
	}
	if onEvent != nil {
		onEvent(contract.ApplyEvent{Format: "zi-setup-event-v1", Phase: phase, Operation: operation, Status: "started", Detail: "synchronizing checkout"})
	}
	return contract.Result{Format: "zi-setup-result-v1", PlanID: f.planID, Phase: phase, Status: "succeeded"}, engine.Output{}, nil
}
