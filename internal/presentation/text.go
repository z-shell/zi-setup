package presentation

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/z-shell/zi-setup/internal/contract"
)

const previewLimit = 8 << 20

func SafeText(value string) string {
	var out strings.Builder
	for len(value) > 0 {
		r, size := utf8.DecodeRuneInString(value)
		if r == utf8.RuneError && size == 1 {
			fmt.Fprintf(&out, "\\x%02x", value[0])
			value = value[1:]
			continue
		}
		value = value[size:]
		switch {
		case r == '\n':
			out.WriteRune(r)
		case r == '\t':
			out.WriteString("    ")
		case r == 0x1b:
			out.WriteString("^[")
		case r < 0x20 || r == 0x7f || r >= 0x80 && r <= 0x9f:
			fmt.Fprintf(&out, "\\u%04x", r)
		default:
			out.WriteRune(r)
		}
	}
	return out.String()
}

func ProfileSummary(profile string) string {
	switch profile {
	case "loader":
		return "Zi only: the loader and readable setup fragments."
	case "annex":
		return "Zi with annexes: the loader plus the verified annex profile."
	case "zunit":
		return "Legacy zunit content is preserved for compatibility."
	default:
		return "Engine-provided profile " + SafeText(profile)
	}
}

func ExperiencePreview(profile string) string {
	var out strings.Builder
	out.WriteString("SIMULATED PREVIEW\n")
	out.WriteString("~/src/zi-demo  main +1\n")
	out.WriteString("% zi status\n")
	switch profile {
	case "annex":
		out.WriteString("Zi loader ready; annex commands become available after first start.\n")
	default:
		out.WriteString("Zi loader ready; no optional profile selected.\n")
	}
	out.WriteString("Synthetic path, Git state, and command. No shell code was run.\n")
	return out.String()
}

func DescribeText(describe contract.Describe) string {
	var out strings.Builder
	out.WriteString("Environment\n")
	for _, fact := range describe.Facts {
		fmt.Fprintf(&out, "  %-18s %s  [%s, %s]\n", fact.ID, SafeText(fact.Value), fact.Source, fact.Confidence)
	}
	out.WriteString("\nProfiles\n")
	for _, profile := range describe.Profiles {
		state := "available"
		if !profile.Selectable {
			state = "unavailable"
		}
		fmt.Fprintf(&out, "  %-8s %-11s %s\n", profile.ID, state, SafeText(profile.Title))
		if profile.Reason != "" {
			fmt.Fprintf(&out, "             %s\n", SafeText(profile.Reason))
		}
	}
	return out.String()
}

func PlanText(plan contract.Plan) string {
	var out strings.Builder
	fmt.Fprintf(&out, "Plan %s\n", plan.ID)
	fmt.Fprintf(&out, "Profile: %s\nRef: %s\nConfig: %s\nCheckout: %s\n\n", plan.Meta.Profile, SafeText(plan.Meta.Ref), SafeText(plan.Meta.ConfigHome), SafeText(plan.Meta.CheckoutPath))
	out.WriteString("Operations\n")
	for _, operation := range plan.Operations {
		fmt.Fprintf(&out, "  %-8s %-12s %s\n", SafeText(operation.Phase), SafeText(operation.Kind), SafeText(operation.Summary))
	}
	if len(plan.Warnings) > 0 {
		out.WriteString("\nWarnings\n")
		for _, warning := range plan.Warnings {
			fmt.Fprintf(&out, "  %s: %s\n", warning.Severity, SafeText(warning.Summary))
			fmt.Fprintf(&out, "    %s\n", SafeText(warning.Remediation))
		}
	}
	out.WriteString("\nTargets\n")
	for _, target := range plan.Targets {
		state := "unchanged"
		if target.Changed {
			state = "change"
		}
		fmt.Fprintf(&out, "  %-8s %-9s %s\n", target.ID, state, SafeText(target.Path))
	}
	return out.String()
}

func GeneratedText(plan contract.Plan) string {
	var out strings.Builder
	for _, target := range plan.Targets {
		fmt.Fprintf(&out, "### %s (%s)\n", target.ID, SafeText(target.Path))
		out.WriteString(SafeText(string(target.Content)))
		if len(target.Content) == 0 || target.Content[len(target.Content)-1] != '\n' {
			out.WriteByte('\n')
		}
		out.WriteByte('\n')
	}
	return out.String()
}

func DiffText(plan contract.Plan) string {
	var out strings.Builder
	for _, target := range plan.Targets {
		if !target.Changed {
			continue
		}
		current, err := currentContent(target)
		if err != nil {
			fmt.Fprintf(&out, "### %s\nDiff unavailable: %s\n\n", SafeText(target.Path), SafeText(err.Error()))
			continue
		}
		fmt.Fprintf(&out, "--- %s (current)\n+++ %s (planned)\n", SafeText(target.Path), SafeText(target.Path))
		out.WriteString(SafeText(lineDiff(string(current), string(target.Content))))
		out.WriteByte('\n')
	}
	if out.Len() == 0 {
		return "No content changes.\n"
	}
	return out.String()
}

func ResultText(checkout, files *contract.Result, verification *contract.Plan) string {
	var out strings.Builder
	out.WriteString("Result\n")
	appendResult := func(label string, result *contract.Result) {
		if result == nil {
			fmt.Fprintf(&out, "  %-9s not run\n", label)
			return
		}
		fmt.Fprintf(&out, "  %-9s %s\n", label, result.Status)
		if result.Error != nil {
			fmt.Fprintf(&out, "    %s: %s\n", SafeText(result.Error.Code), SafeText(result.Error.Detail))
		}
		if result.ReceiptPath != "" {
			fmt.Fprintf(&out, "    receipt: %s\n", SafeText(result.ReceiptPath))
		}
	}
	appendResult("checkout", checkout)
	appendResult("files", files)
	if verification != nil {
		if verification.HasContentChanges() {
			out.WriteString("  reopen: content changes remain\n")
		} else {
			out.WriteString("  reopen: no content changes\n")
		}
	}
	return out.String()
}

func currentContent(target contract.Target) ([]byte, error) {
	root, err := os.OpenRoot(filepath.Dir(target.Path))
	if errorsIsNotExist(err) && target.Expected == "missing" {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer root.Close()
	name := filepath.Base(target.Path)
	info, err := root.Lstat(name)
	if errorsIsNotExist(err) && target.Expected == "missing" {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("target is a symlink")
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("target is not a regular file")
	}
	if info.Size() > previewLimit {
		return nil, fmt.Errorf("target exceeds preview limit")
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !os.SameFile(info, openedInfo) || !openedInfo.Mode().IsRegular() {
		return nil, fmt.Errorf("target changed while opening; create a new plan")
	}
	data, err := io.ReadAll(io.LimitReader(file, previewLimit+1))
	if err != nil {
		return nil, err
	}
	if len(data) > previewLimit {
		return nil, fmt.Errorf("target exceeds preview limit")
	}
	digest := sha256.Sum256(data)
	if hex.EncodeToString(digest[:]) != target.Expected {
		return nil, fmt.Errorf("target changed after planning; create a new plan")
	}
	return data, nil
}

func errorsIsNotExist(err error) bool { return err != nil && os.IsNotExist(err) }

func lineDiff(oldText, newText string) string {
	oldLines := splitLines(oldText)
	newLines := splitLines(newText)
	if len(oldLines) > 4000 || len(newLines) > 4000 || len(oldLines)*len(newLines) > 4_000_000 {
		return "[diff omitted: text is too large for an interactive preview]\n"
	}
	lcs := make([][]int, len(oldLines)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(newLines)+1)
	}
	for i := len(oldLines) - 1; i >= 0; i-- {
		for j := len(newLines) - 1; j >= 0; j-- {
			if oldLines[i] == newLines[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}
	var out strings.Builder
	for i, j := 0, 0; i < len(oldLines) || j < len(newLines); {
		switch {
		case i < len(oldLines) && j < len(newLines) && oldLines[i] == newLines[j]:
			out.WriteString(" " + oldLines[i] + "\n")
			i++
			j++
		case j == len(newLines) || i < len(oldLines) && lcs[i+1][j] >= lcs[i][j+1]:
			out.WriteString("-" + oldLines[i] + "\n")
			i++
		default:
			out.WriteString("+" + newLines[j] + "\n")
			j++
		}
	}
	return out.String()
}

func splitLines(value string) []string {
	if value == "" {
		return nil
	}
	lines := strings.Split(value, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
