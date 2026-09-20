package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/z-shell/zi-setup/internal/contract"
)

const outputLimit = 1 << 20

type Inputs struct {
	Home       string
	ConfigHome string
	Zshrc      string
	ZiHome     string
	ZiBinDir   string
	Ref        string
	Init       string
	Profiles   string
	Checksum   string
	SkipZshrc  bool
}

type Client struct {
	EnginePath string
	ShellPath  string
	TempParent string
}

type Output struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

type CommandError struct {
	Command  string
	ExitCode int
	Stderr   string
	Err      error
}

func (e *CommandError) Error() string {
	if e.Stderr != "" {
		return fmt.Sprintf("%s exited %d: %s", e.Command, e.ExitCode, e.Stderr)
	}
	return fmt.Sprintf("%s exited %d: %v", e.Command, e.ExitCode, e.Err)
}

func (e *CommandError) Unwrap() error { return e.Err }

type Workspace struct {
	client   Client
	inputs   Inputs
	root     string
	sequence int
	planPath string
	plan     contract.Plan
}

func (c Client) NewWorkspace(inputs Inputs) (*Workspace, error) {
	if c.EnginePath == "" {
		return nil, errors.New("engine path is required")
	}
	enginePath, err := filepath.Abs(c.EnginePath)
	if err != nil {
		return nil, fmt.Errorf("resolve engine path: %w", err)
	}
	info, err := os.Stat(enginePath)
	if err != nil {
		return nil, fmt.Errorf("stat engine: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("engine is not a regular file: %s", enginePath)
	}
	c.EnginePath = enginePath
	if c.ShellPath == "" {
		c.ShellPath = "sh"
	}
	root, err := os.MkdirTemp(c.TempParent, "zi-setup-")
	if err != nil {
		return nil, fmt.Errorf("create private artifact root: %w", err)
	}
	if err := os.Chmod(root, 0o700); err != nil {
		os.RemoveAll(root)
		return nil, fmt.Errorf("restrict artifact root: %w", err)
	}
	return &Workspace{client: c, inputs: inputs, root: root}, nil
}

func (w *Workspace) Close() error {
	if w.root == "" {
		return nil
	}
	err := os.RemoveAll(w.root)
	w.root = ""
	return err
}

func (w *Workspace) Root() string { return w.root }

func (w *Workspace) Describe(ctx context.Context) (contract.Describe, Output, error) {
	path := w.nextPath("describe")
	args := []string{"describe", "--output", path}
	args = append(args, describeArgs(w.inputs)...)
	output, runErr := w.run(ctx, args)
	describe, readErr := contract.ReadDescribe(path)
	if readErr != nil {
		if runErr != nil {
			return contract.Describe{}, output, runErr
		}
		return contract.Describe{}, output, fmt.Errorf("read describe artifact: %w", readErr)
	}
	return describe, output, runErr
}

func (w *Workspace) Plan(ctx context.Context, profile string) (contract.Plan, Output, error) {
	path := w.nextPath("plan")
	args := []string{"plan", "--plan", path, "--profile", profile}
	args = append(args, planArgs(w.inputs)...)
	output, runErr := w.run(ctx, args)
	if runErr != nil {
		return contract.Plan{}, output, runErr
	}
	plan, err := contract.ReadPlan(path)
	if err != nil {
		return contract.Plan{}, output, fmt.Errorf("read plan artifact: %w", err)
	}
	w.planPath = path
	w.plan = plan
	return plan, output, nil
}

func (w *Workspace) Apply(ctx context.Context, phase string) (contract.Result, Output, error) {
	if w.planPath == "" || w.plan.ID == "" {
		return contract.Result{}, Output{}, errors.New("no reviewed plan is available")
	}
	if phase != "checkout" && phase != "files" {
		return contract.Result{}, Output{}, fmt.Errorf("unsupported phase %q", phase)
	}
	resultPath := w.nextPath("result-" + phase)
	args := []string{"apply", "--plan", w.planPath, "--phase", phase, "--expect", w.plan.ID, "--result", resultPath}
	output, runErr := w.run(ctx, args)
	result, readErr := contract.ReadResult(resultPath)
	if readErr != nil {
		if runErr != nil {
			return contract.Result{}, output, runErr
		}
		return contract.Result{}, output, fmt.Errorf("read %s result artifact: %w", phase, readErr)
	}
	if result.PlanID != w.plan.ID {
		return contract.Result{}, output, fmt.Errorf("%s result references plan %q, expected %q", phase, result.PlanID, w.plan.ID)
	}
	if result.Phase != phase {
		return contract.Result{}, output, fmt.Errorf("%s apply returned a %q result", phase, result.Phase)
	}
	return result, output, runErr
}

func (w *Workspace) nextPath(prefix string) string {
	w.sequence++
	return filepath.Join(w.root, prefix+"-"+strconv.Itoa(w.sequence))
}

func describeArgs(inputs Inputs) []string {
	var args []string
	args = appendValue(args, "--zi-home", inputs.ZiHome)
	args = appendValue(args, "--zi-bin-dir", inputs.ZiBinDir)
	args = appendValue(args, "--config-home", inputs.ConfigHome)
	args = appendValue(args, "--zshrc", inputs.Zshrc)
	args = appendValue(args, "--profiles", inputs.Profiles)
	if inputs.SkipZshrc {
		args = append(args, "--skip-zshrc")
	}
	return args
}

func planArgs(inputs Inputs) []string {
	var args []string
	args = appendValue(args, "--ref", inputs.Ref)
	args = appendValue(args, "--zi-home", inputs.ZiHome)
	args = appendValue(args, "--zi-bin-dir", inputs.ZiBinDir)
	args = appendValue(args, "--config-home", inputs.ConfigHome)
	args = appendValue(args, "--zshrc", inputs.Zshrc)
	args = appendValue(args, "--init", inputs.Init)
	args = appendValue(args, "--profiles", inputs.Profiles)
	args = appendValue(args, "--checksum", inputs.Checksum)
	if inputs.SkipZshrc {
		args = append(args, "--skip-zshrc")
	}
	return args
}

func appendValue(args []string, name, value string) []string {
	if value == "" {
		return args
	}
	return append(args, name, value)
}

func (w *Workspace) run(ctx context.Context, args []string) (Output, error) {
	commandArgs := append([]string{w.client.EnginePath}, args...)
	cmd := exec.CommandContext(ctx, w.client.ShellPath, commandArgs...)
	cmd.Env = environment(w.inputs.Home)
	var stdout, stderr limitedBuffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	output := Output{Stdout: stdout.String(), Stderr: stderr.String()}
	if err == nil {
		return output, nil
	}
	exitCode := 1
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		exitCode = exitErr.ExitCode()
	} else if ctx.Err() != nil {
		exitCode = 6
	}
	output.ExitCode = exitCode
	return output, &CommandError{Command: args[0], ExitCode: exitCode, Stderr: strings.TrimSpace(output.Stderr), Err: err}
}

func environment(home string) []string {
	if home == "" {
		return os.Environ()
	}
	result := make([]string, 0, len(os.Environ())+1)
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "HOME=") {
			result = append(result, entry)
		}
	}
	return append(result, "HOME="+home)
}

type limitedBuffer struct {
	buffer    bytes.Buffer
	truncated bool
}

func (b *limitedBuffer) Write(data []byte) (int, error) {
	original := len(data)
	remaining := outputLimit - b.buffer.Len()
	if remaining > 0 {
		if len(data) > remaining {
			data = data[:remaining]
			b.truncated = true
		}
		_, _ = b.buffer.Write(data)
	} else {
		b.truncated = true
	}
	return original, nil
}

func (b *limitedBuffer) String() string {
	if b.truncated {
		return b.buffer.String() + "\n[output truncated]\n"
	}
	return b.buffer.String()
}
