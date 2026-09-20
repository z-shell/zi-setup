package workflow

import (
	"context"
	"errors"
	"fmt"

	"github.com/z-shell/zi-setup/internal/contract"
	"github.com/z-shell/zi-setup/internal/engine"
)

type Stage string

const (
	StageDiscover Stage = "discover"
	StageChoose   Stage = "choose"
	StageReview   Stage = "review"
	StageApply    Stage = "apply"
	StageResult   Stage = "result"
)

type Engine interface {
	Describe(context.Context) (contract.Describe, engine.Output, error)
	Plan(context.Context, string) (contract.Plan, engine.Output, error)
	Apply(context.Context, string) (contract.Result, engine.Output, error)
}

type PhaseOutcome struct {
	Result contract.Result
	Output engine.Output
	Err    error
}

type Session struct {
	engine           Engine
	Stage            Stage
	Describe         contract.Describe
	DiscoveryOutput  engine.Output
	DiscoveryErr     error
	SelectedProfile  string
	Plan             contract.Plan
	PlanOutput       engine.Output
	Checkout         *PhaseOutcome
	Files            *PhaseOutcome
	VerificationPlan *contract.Plan
}

func New(setupEngine Engine) *Session {
	return &Session{engine: setupEngine, Stage: StageDiscover}
}

// Clone returns an independent presentation-state copy that shares the engine.
// TUI commands mutate the copy off the render loop and publish it only when the
// command completes.
func (s *Session) Clone() *Session {
	clone := *s
	return &clone
}

func (s *Session) DiscoverEnvironment(ctx context.Context) error {
	describe, output, err := s.engine.Describe(ctx)
	s.Describe = describe
	s.DiscoveryOutput = output
	s.DiscoveryErr = err
	if describe.Format != "" {
		s.Stage = StageChoose
	}
	return err
}

func (s *Session) SelectProfile(profile string) error {
	if s.Describe.Format == "" {
		return errors.New("discovery has not completed")
	}
	choice, ok := s.Describe.Profile(profile)
	if !ok {
		return fmt.Errorf("profile %q was not offered by the engine", profile)
	}
	if !choice.Selectable {
		if choice.Reason != "" {
			return fmt.Errorf("profile %q is unavailable: %s", profile, choice.Reason)
		}
		return fmt.Errorf("profile %q is unavailable", profile)
	}
	s.SelectedProfile = profile
	s.Plan = contract.Plan{}
	s.Checkout = nil
	s.Files = nil
	s.VerificationPlan = nil
	s.Stage = StageChoose
	return nil
}

func (s *Session) BuildPlan(ctx context.Context) error {
	if s.SelectedProfile == "" {
		return errors.New("a selectable profile is required")
	}
	plan, output, err := s.engine.Plan(ctx, s.SelectedProfile)
	s.PlanOutput = output
	if err != nil {
		return err
	}
	s.Plan = plan
	s.Checkout = nil
	s.Files = nil
	s.VerificationPlan = nil
	s.Stage = StageReview
	return nil
}

func (s *Session) BackToChoose() {
	s.Plan = contract.Plan{}
	s.PlanOutput = engine.Output{}
	s.Checkout = nil
	s.Files = nil
	s.VerificationPlan = nil
	s.Stage = StageChoose
}

func (s *Session) ApplyReviewedPlan(ctx context.Context) error {
	if s.Plan.ID == "" || s.Stage != StageReview {
		return errors.New("a reviewed plan is required")
	}
	s.Stage = StageApply
	checkout, output, err := s.engine.Apply(ctx, "checkout")
	s.Checkout = &PhaseOutcome{Result: checkout, Output: output, Err: err}
	if err != nil || checkout.Status != "succeeded" {
		s.Stage = StageResult
		if err != nil {
			return err
		}
		return errors.New("checkout phase did not succeed")
	}
	files, output, err := s.engine.Apply(ctx, "files")
	s.Files = &PhaseOutcome{Result: files, Output: output, Err: err}
	s.Stage = StageResult
	if err != nil {
		return err
	}
	if files.Status != "succeeded" {
		return errors.New("files phase did not succeed")
	}
	return nil
}

func (s *Session) VerifyReopen(ctx context.Context) error {
	if s.Stage != StageResult || s.Files == nil || s.Files.Result.Status != "succeeded" {
		return errors.New("successful application is required before verification")
	}
	plan, _, err := s.engine.Plan(ctx, s.SelectedProfile)
	if err != nil {
		return err
	}
	s.VerificationPlan = &plan
	return nil
}

func (s *Session) ReopenHasNoContentChanges() bool {
	return s.VerificationPlan != nil && !s.VerificationPlan.HasContentChanges()
}
