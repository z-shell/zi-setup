package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/z-shell/zi-setup/internal/engine"
	"github.com/z-shell/zi-setup/internal/plain"
	"github.com/z-shell/zi-setup/internal/presentation"
	"github.com/z-shell/zi-setup/internal/tui"
	"github.com/z-shell/zi-setup/internal/workflow"
	"golang.org/x/term"
)

var version = "dev"

type options struct {
	enginePath   string
	engineEvents bool
	shellPath    string
	plain        bool
	headless     bool
	profile      string
	apply        bool
	yes          bool
	theme        string
	noColor      bool
	ascii        bool
	showVersion  bool
	inputs       engine.Inputs
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(arguments []string) int {
	options, err := parseFlags(arguments)
	if err != nil {
		fmt.Fprintln(os.Stderr, presentation.SafeText(err.Error()))
		return 2
	}
	if options.showVersion {
		fmt.Printf("zi-setup %s\nengine %s\n", version, engine.BundledEngineRevision)
		return 0
	}
	client := engine.Client{EnginePath: options.enginePath, ShellPath: options.shellPath, Events: options.engineEvents}
	workspace, err := client.NewWorkspace(options.inputs)
	if err != nil {
		fmt.Fprintln(os.Stderr, presentation.SafeText(err.Error()))
		return 2
	}
	defer workspace.Close()
	session := workflow.New(workspace, workflow.Options{Ref: options.inputs.Ref, SkipZshrc: options.inputs.SkipZshrc})
	ctx := context.Background()
	linear := useLinear(options, term.IsTerminal(int(os.Stdin.Fd())), term.IsTerminal(int(os.Stdout.Fd())))
	if linear {
		err = plain.Run(ctx, session, plain.Options{
			Profile: options.profile,
			Apply:   options.apply,
			Yes:     options.yes,
			Input:   os.Stdin,
			Output:  os.Stdout,
		})
	} else {
		err = tui.Run(ctx, session, tui.Options{Theme: options.theme, NoColor: options.noColor, ASCII: options.ascii})
	}
	if err == nil {
		return 0
	}
	fmt.Fprintln(os.Stderr, presentation.SafeText(err.Error()))
	var commandErr *engine.CommandError
	if errors.As(err, &commandErr) && commandErr.ExitCode >= 2 && commandErr.ExitCode <= 6 {
		return commandErr.ExitCode
	}
	return 1
}

func useLinear(options options, inputTerminal, outputTerminal bool) bool {
	return options.plain || options.headless || options.profile != "" || options.apply || options.yes || !inputTerminal || !outputTerminal
}

func parseFlags(arguments []string) (options, error) {
	var result options
	flags := flag.NewFlagSet("zi-setup", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	flags.StringVar(&result.enginePath, "engine", os.Getenv("ZI_SETUP_ENGINE"), "path to public/sh/setup.sh")
	flags.BoolVar(&result.engineEvents, "engine-events", false, "enable zi-setup-event-v1 for an external engine")
	flags.StringVar(&result.shellPath, "shell", "sh", "POSIX shell used to invoke the engine")
	flags.BoolVar(&result.plain, "plain", false, "use linear interactive output")
	flags.BoolVar(&result.headless, "headless", false, "run without terminal control; requires --profile")
	flags.StringVar(&result.profile, "profile", "", "engine profile: loader or annex")
	flags.BoolVar(&result.apply, "apply", false, "apply the reviewed plan")
	flags.BoolVar(&result.yes, "yes", false, "approve the exact plan without a prompt; requires --profile and --apply")
	flags.StringVar(&result.theme, "theme", "dark", "theme: dark, light, or mono")
	flags.BoolVar(&result.noColor, "no-color", os.Getenv("NO_COLOR") != "", "disable color")
	flags.BoolVar(&result.ascii, "ascii", false, "use ASCII-only interface markers")
	flags.BoolVar(&result.showVersion, "version", false, "print version")
	flags.StringVar(&result.inputs.Home, "home", "", "HOME passed to the setup engine")
	flags.StringVar(&result.inputs.ConfigHome, "config-home", "", "explicit Zi configuration home")
	flags.StringVar(&result.inputs.Zshrc, "zshrc", "", "explicit Zsh startup file")
	flags.StringVar(&result.inputs.ZiHome, "zi-home", "", "explicit Zi data home")
	flags.StringVar(&result.inputs.ZiBinDir, "zi-bin-dir", "", "explicit Zi checkout directory name")
	flags.StringVar(&result.inputs.Ref, "ref", "", "Zi branch or tag")
	flags.StringVar(&result.inputs.Init, "init", "", "engine init.zsh asset")
	flags.StringVar(&result.inputs.Profiles, "profiles", "", "engine profiles.tsv asset")
	flags.StringVar(&result.inputs.Checksum, "checksum", "", "engine checksum manifest")
	flags.BoolVar(&result.inputs.SkipZshrc, "skip-zshrc", false, "leave .zshrc unchanged")
	if err := flags.Parse(arguments); err != nil {
		return options{}, err
	}
	if flags.NArg() != 0 {
		return options{}, fmt.Errorf("unexpected arguments: %v", flags.Args())
	}
	if result.showVersion {
		return result, nil
	}
	switch result.profile {
	case "", "loader", "annex":
	default:
		return options{}, fmt.Errorf("--profile must be loader or annex")
	}
	switch result.theme {
	case "dark", "light", "mono":
	default:
		return options{}, fmt.Errorf("--theme must be dark, light, or mono")
	}
	if result.headless && result.profile == "" {
		return options{}, fmt.Errorf("--headless requires --profile")
	}
	if result.yes && (!result.apply || result.profile == "") {
		return options{}, fmt.Errorf("--yes requires --apply and --profile")
	}
	return result, nil
}
