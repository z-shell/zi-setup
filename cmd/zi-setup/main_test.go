package main

import "testing"

func TestParseFlagsUsesBundledEngineByDefault(t *testing.T) {
	t.Setenv("ZI_SETUP_ENGINE", "")
	got, err := parseFlags(nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.enginePath != "" {
		t.Fatalf("engine path = %q, want bundled engine", got.enginePath)
	}
}

func TestUseLinearHonorsCommandLineIntent(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		options options
		stdin   bool
		stdout  bool
		want    bool
	}{
		{name: "default terminals use tui", stdin: true, stdout: true},
		{name: "plain", options: options{plain: true}, stdin: true, stdout: true, want: true},
		{name: "headless", options: options{headless: true}, stdin: true, stdout: true, want: true},
		{name: "profile", options: options{profile: "loader"}, stdin: true, stdout: true, want: true},
		{name: "apply", options: options{apply: true}, stdin: true, stdout: true, want: true},
		{name: "yes", options: options{yes: true}, stdin: true, stdout: true, want: true},
		{name: "redirected input", stdout: true, want: true},
		{name: "redirected output", stdin: true, want: true},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := useLinear(test.options, test.stdin, test.stdout); got != test.want {
				t.Fatalf("useLinear() = %v, want %v", got, test.want)
			}
		})
	}
}
