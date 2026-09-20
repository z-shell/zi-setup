package plain

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/z-shell/zi-setup/internal/contract"
	"github.com/z-shell/zi-setup/internal/presentation"
	"github.com/z-shell/zi-setup/internal/workflow"
)

type Options struct {
	Profile string
	Apply   bool
	Yes     bool
	Input   io.Reader
	Output  io.Writer
}

func Run(ctx context.Context, session *workflow.Session, options Options) error {
	if options.Input == nil {
		options.Input = strings.NewReader("")
	}
	if options.Output == nil {
		options.Output = io.Discard
	}
	reader := bufio.NewReader(options.Input)

	discoveryErr := session.DiscoverEnvironment(ctx)
	fmt.Fprintln(options.Output, presentation.DescribeText(session.Describe))
	if discoveryErr != nil {
		return discoveryErr
	}
	profile := options.Profile
	if profile == "" {
		var err error
		profile, err = chooseProfile(reader, options.Output, session.Describe.Profiles)
		if err != nil {
			return err
		}
	}
	if err := session.SelectProfile(profile); err != nil {
		return err
	}
	if err := session.BuildPlan(ctx); err != nil {
		return err
	}
	fmt.Fprintln(options.Output, presentation.ProfileSummary(session.Plan.Meta.Profile))
	fmt.Fprintln(options.Output, presentation.PlanText(session.Plan))
	fmt.Fprintln(options.Output, "Generated Zsh")
	fmt.Fprintln(options.Output, presentation.GeneratedText(session.Plan))
	fmt.Fprintln(options.Output, "File changes")
	fmt.Fprintln(options.Output, presentation.DiffText(session.Plan))
	if !options.Apply {
		fmt.Fprintf(options.Output, "Review complete. Re-run with --apply to apply plan %s.\n", session.Plan.ID)
		return nil
	}
	if !options.Yes {
		fmt.Fprintf(options.Output, "Type apply %s to approve this exact plan: ", session.Plan.ID)
		line, err := reader.ReadString('\n')
		if err != nil && len(line) == 0 {
			return fmt.Errorf("read approval: %w", err)
		}
		if strings.TrimSpace(line) != "apply "+session.Plan.ID {
			return fmt.Errorf("plan was not approved")
		}
	}
	applyErr := session.ApplyReviewedPlan(ctx)
	if applyErr == nil {
		if err := session.VerifyReopen(ctx); err != nil {
			applyErr = fmt.Errorf("verify reopen: %w", err)
		}
	}
	var checkout, files *contract.Result
	if session.Checkout != nil {
		checkout = &session.Checkout.Result
	}
	if session.Files != nil {
		files = &session.Files.Result
	}
	fmt.Fprintln(options.Output, presentation.ResultText(checkout, files, session.VerificationPlan))
	return applyErr
}

func chooseProfile(reader *bufio.Reader, output io.Writer, profiles []contract.Profile) (string, error) {
	var choices []contract.Profile
	for _, profile := range profiles {
		if profile.Selectable {
			choices = append(choices, profile)
		}
	}
	if len(choices) == 0 {
		return "", fmt.Errorf("the engine did not offer an actionable profile")
	}
	for i, choice := range choices {
		fmt.Fprintf(output, "%d. %s: %s\n", i+1, presentation.SafeText(choice.Title), presentation.ProfileSummary(choice.ID))
	}
	fmt.Fprintf(output, "Choose a profile [1-%d]: ", len(choices))
	line, err := reader.ReadString('\n')
	if err != nil && len(line) == 0 {
		return "", fmt.Errorf("read profile choice: %w", err)
	}
	for i := range choices {
		if strings.TrimSpace(line) == fmt.Sprint(i+1) {
			return choices[i].ID, nil
		}
	}
	return "", fmt.Errorf("invalid profile choice %q", strings.TrimSpace(line))
}
