package contract

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const (
	describeFormat = "zi-setup-describe-v1"
	planFormat     = "zi-setup-plan-v1"
	resultFormat   = "zi-setup-result-v1"
	eventFormat    = "zi-setup-event-v1"
	metadataLimit  = 64 << 10
	contentLimit   = 8 << 20
)

var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

type reader struct {
	root *os.Root
}

func openReader(path string) (*reader, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("open artifact: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, fmt.Errorf("artifact root must be a directory, got %s", info.Mode())
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, fmt.Errorf("open artifact root: %w", err)
	}
	return &reader{root: root}, nil
}

func (r *reader) close() error { return r.root.Close() }

func (r *reader) bytes(name string, limit int64) ([]byte, error) {
	f, err := r.root.Open(name)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", name, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", name)
	}
	if info.Size() > limit {
		return nil, fmt.Errorf("%s exceeds %d bytes", name, limit)
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s exceeds %d bytes", name, limit)
	}
	return data, nil
}

func (r *reader) raw(name string) ([]byte, error) {
	return r.bytes(name, contentLimit)
}

func (r *reader) text(name string) (string, error) {
	data, err := r.bytes(name, metadataLimit)
	if err != nil {
		return "", err
	}
	text := strings.TrimSuffix(string(data), "\n")
	if strings.ContainsAny(text, "\n\r\t\x00") {
		return "", fmt.Errorf("%s is not a restricted single-line value", name)
	}
	return text, nil
}

func (r *reader) optionalText(name string) (string, bool, error) {
	value, err := r.text(name)
	if err == nil {
		return value, true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	return "", false, err
}

func (r *reader) order(name string, emptyOK bool) ([]string, error) {
	data, err := r.bytes(name, metadataLimit)
	if err != nil {
		return nil, err
	}
	text := strings.TrimSuffix(string(data), "\n")
	if text == "" {
		if emptyOK {
			return nil, nil
		}
		return nil, fmt.Errorf("%s is empty", name)
	}
	items := strings.Split(text, "\n")
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		if !idPattern.MatchString(item) {
			return nil, fmt.Errorf("%s contains invalid id %q", name, item)
		}
		if _, exists := seen[item]; exists {
			return nil, fmt.Errorf("%s contains duplicate id %q", name, item)
		}
		seen[item] = struct{}{}
	}
	return items, nil
}

func requireOneOf(field, value string, allowed ...string) error {
	for _, candidate := range allowed {
		if value == candidate {
			return nil
		}
	}
	return fmt.Errorf("%s has unsupported value %q", field, value)
}

func requireSHA256(field, value string) error {
	if len(value) != sha256.Size*2 {
		return fmt.Errorf("%s is not a SHA-256 value", field)
	}
	if _, err := hex.DecodeString(value); err != nil {
		return fmt.Errorf("%s is not a SHA-256 value: %w", field, err)
	}
	return nil
}

func validateFactValue(id, value string) error {
	allowed := map[string][]string{
		"zi-home-state":    {"selected", "ambiguous"},
		"zshrc-state":      {"skipped", "symlink", "file", "other", "missing"},
		"git":              {"available", "missing"},
		"zsh":              {"available", "missing"},
		"tty":              {"yes", "no"},
		"existing-profile": {"loader", "annex", "zunit", "none"},
	}
	values, restricted := allowed[id]
	if !restricted {
		return nil
	}
	return requireOneOf("facts/"+id+"/value", value, values...)
}

func ReadDescribe(path string) (Describe, error) {
	r, err := openReader(path)
	if err != nil {
		return Describe{}, err
	}
	defer r.close()
	format, err := r.text("format")
	if err != nil {
		return Describe{}, err
	}
	if format != describeFormat {
		return Describe{}, fmt.Errorf("unsupported describe format %q", format)
	}
	factIDs, err := r.order("facts/order", false)
	if err != nil {
		return Describe{}, err
	}
	expectedFacts := []string{"config-home", "zi-home", "checkout-path", "zi-home-state", "zshrc-path", "zshrc-state", "git", "zsh", "tty", "existing-profile"}
	if strings.Join(factIDs, "\n") != strings.Join(expectedFacts, "\n") {
		return Describe{}, fmt.Errorf("describe facts/order does not match %s", describeFormat)
	}
	describe := Describe{Format: format}
	for _, id := range factIDs {
		base := "facts/" + id + "/"
		value, err := r.text(base + "value")
		if err != nil {
			return Describe{}, err
		}
		if err := validateFactValue(id, value); err != nil {
			return Describe{}, err
		}
		source, err := r.text(base + "source")
		if err != nil {
			return Describe{}, err
		}
		if err := requireOneOf(base+"source", source, "observed", "inferred", "confirmed", "unknown"); err != nil {
			return Describe{}, err
		}
		confidence, err := r.text(base + "confidence")
		if err != nil {
			return Describe{}, err
		}
		if err := requireOneOf(base+"confidence", confidence, "certain", "likely", "unknown"); err != nil {
			return Describe{}, err
		}
		describe.Facts = append(describe.Facts, Fact{ID: id, Value: value, Source: source, Confidence: confidence})
	}
	profileIDs, err := r.order("profiles/order", false)
	if err != nil {
		return Describe{}, err
	}
	if len(profileIDs) < 2 || len(profileIDs) > 3 || profileIDs[0] != "loader" || profileIDs[1] != "annex" || len(profileIDs) == 3 && profileIDs[2] != "zunit" {
		return Describe{}, fmt.Errorf("profiles/order does not match %s", describeFormat)
	}
	for _, id := range profileIDs {
		base := "profiles/" + id + "/"
		selectableText, err := r.text(base + "selectable")
		if err != nil {
			return Describe{}, err
		}
		if err := requireOneOf(base+"selectable", selectableText, "yes", "no"); err != nil {
			return Describe{}, err
		}
		if id == "zunit" && selectableText != "no" {
			return Describe{}, fmt.Errorf("%s compatibility profile must not be selectable", base)
		}
		reason, err := r.text(base + "reason")
		if err != nil {
			return Describe{}, err
		}
		title, err := r.text(base + "title")
		if err != nil {
			return Describe{}, err
		}
		describe.Profiles = append(describe.Profiles, Profile{ID: id, Selectable: selectableText == "yes", Reason: reason, Title: title})
	}
	return describe, nil
}

func readMeta(r *reader) (map[string]string, error) {
	data, err := r.bytes("plan.meta", metadataLimit)
	if err != nil {
		return nil, err
	}
	values := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok || key == "" || strings.ContainsAny(key, "\t\r \x00") || strings.ContainsAny(value, "\t\r\x00") {
			return nil, fmt.Errorf("plan.meta contains invalid line %q", line)
		}
		if _, exists := values[key]; exists {
			return nil, fmt.Errorf("plan.meta contains duplicate key %q", key)
		}
		values[key] = value
	}
	for _, key := range []string{"format", "profile", "ref", "config_home", "checkout_path", "receipt_path", "skip_zshrc"} {
		if _, ok := values[key]; !ok {
			return nil, fmt.Errorf("plan.meta is missing %q", key)
		}
	}
	if len(values) != 7 {
		return nil, fmt.Errorf("plan.meta contains unknown keys")
	}
	return values, nil
}

func ReadPlan(path string) (Plan, error) {
	r, err := openReader(path)
	if err != nil {
		return Plan{}, err
	}
	defer r.close()
	meta, err := readMeta(r)
	if err != nil {
		return Plan{}, err
	}
	if meta["format"] != planFormat {
		return Plan{}, fmt.Errorf("unsupported plan format %q", meta["format"])
	}
	if err := requireOneOf("profile", meta["profile"], "loader", "annex", "zunit"); err != nil {
		return Plan{}, err
	}
	skipZshrc, err := strconv.ParseBool(map[string]string{"0": "false", "1": "true"}[meta["skip_zshrc"]])
	if err != nil {
		return Plan{}, fmt.Errorf("skip_zshrc must be 0 or 1")
	}
	planID, err := r.text("plan.id")
	if err != nil {
		return Plan{}, err
	}
	if err := requireSHA256("plan.id", planID); err != nil {
		return Plan{}, err
	}
	plan := Plan{
		Format: planFormat,
		ID:     planID,
		Meta: PlanMeta{
			Profile:      meta["profile"],
			Ref:          meta["ref"],
			ConfigHome:   meta["config_home"],
			CheckoutPath: meta["checkout_path"],
			ReceiptPath:  meta["receipt_path"],
			SkipZshrc:    skipZshrc,
		},
	}
	for name, destination := range map[string]*string{
		"checkout/kind":          &plan.Checkout.Kind,
		"checkout/head":          &plan.Checkout.Head,
		"checkout/current-ref":   &plan.Checkout.CurrentRef,
		"checkout/origin":        &plan.Checkout.Origin,
		"checkout/requested-ref": &plan.Checkout.RequestedRef,
	} {
		*destination, err = r.text(name)
		if err != nil {
			return Plan{}, err
		}
	}
	if err := requireOneOf("checkout/kind", plan.Checkout.Kind, "missing", "existing"); err != nil {
		return Plan{}, err
	}
	targetIDs, err := r.order("targets/order", false)
	if err != nil {
		return Plan{}, err
	}
	for _, id := range targetIDs {
		base := "targets/" + id + "/"
		target := Target{ID: id}
		for name, destination := range map[string]*string{"path": &target.Path, "kind": &target.Kind, "expected": &target.Expected, "mode": &target.Mode} {
			*destination, err = r.text(base + name)
			if err != nil {
				return Plan{}, err
			}
		}
		if !filepath.IsAbs(target.Path) || filepath.Clean(target.Path) != target.Path {
			return Plan{}, fmt.Errorf("%s path must be clean and absolute", base)
		}
		target.Content, err = r.raw(base + "content")
		if err != nil {
			return Plan{}, err
		}
		target.BlockHash, _, err = r.optionalText(base + "block-hash")
		if err != nil {
			return Plan{}, err
		}
		digest := sha256.Sum256(target.Content)
		target.Changed = target.Expected != hex.EncodeToString(digest[:])
		plan.Targets = append(plan.Targets, target)
	}
	operationIDs, err := r.order("operations/order", false)
	if err != nil {
		return Plan{}, err
	}
	if strings.Join(operationIDs, "\n") != "checkout-sync\nwrite-files" {
		return Plan{}, fmt.Errorf("operations/order does not match %s", planFormat)
	}
	for _, id := range operationIDs {
		base := "operations/" + id + "/"
		operation := Operation{ID: id}
		for name, destination := range map[string]*string{"phase": &operation.Phase, "kind": &operation.Kind, "summary": &operation.Summary} {
			*destination, err = r.text(base + name)
			if err != nil {
				return Plan{}, err
			}
		}
		interruptible, err := r.text(base + "interruptible")
		if err != nil {
			return Plan{}, err
		}
		if err := requireOneOf(base+"interruptible", interruptible, "yes", "no"); err != nil {
			return Plan{}, err
		}
		operation.Interruptible = interruptible == "yes"
		switch id {
		case "checkout-sync":
			if operation.Phase != "checkout" || operation.Interruptible {
				return Plan{}, fmt.Errorf("%s does not match %s", base, planFormat)
			}
			if err := requireOneOf(base+"kind", operation.Kind, "clone", "fast-forward"); err != nil {
				return Plan{}, err
			}
		case "write-files":
			if operation.Phase != "files" || operation.Kind != "write-files" || operation.Interruptible {
				return Plan{}, fmt.Errorf("%s does not match %s", base, planFormat)
			}
		}
		plan.Operations = append(plan.Operations, operation)
	}
	warningIDs, err := r.order("warnings/order", true)
	if err != nil {
		return Plan{}, err
	}
	for _, id := range warningIDs {
		base := "warnings/" + id + "/"
		warning := Warning{ID: id}
		for name, destination := range map[string]*string{"severity": &warning.Severity, "summary": &warning.Summary, "remediation": &warning.Remediation} {
			*destination, err = r.text(base + name)
			if err != nil {
				return Plan{}, err
			}
		}
		if err := requireOneOf(base+"severity", warning.Severity, "info", "warning", "critical"); err != nil {
			return Plan{}, err
		}
		plan.Warnings = append(plan.Warnings, warning)
	}
	return plan, nil
}

func ReadResult(path string) (Result, error) {
	r, err := openReader(path)
	if err != nil {
		return Result{}, err
	}
	defer r.close()
	format, err := r.text("format")
	if err != nil {
		return Result{}, err
	}
	if format != resultFormat {
		return Result{}, fmt.Errorf("unsupported result format %q", format)
	}
	result := Result{Format: format}
	for name, destination := range map[string]*string{"plan.id": &result.PlanID, "phase": &result.Phase, "status": &result.Status} {
		*destination, err = r.text(name)
		if err != nil {
			return Result{}, err
		}
	}
	if err := requireSHA256("plan.id", result.PlanID); err != nil {
		return Result{}, err
	}
	if err := requireOneOf("phase", result.Phase, "checkout", "files"); err != nil {
		return Result{}, err
	}
	if err := requireOneOf("status", result.Status, "succeeded", "failed", "cancelled"); err != nil {
		return Result{}, err
	}
	operationIDs, err := r.order("operations/order", true)
	if err != nil {
		return Result{}, err
	}
	expectedOperation := map[string]string{"checkout": "checkout-sync", "files": "write-files"}[result.Phase]
	if len(operationIDs) > 1 || len(operationIDs) == 1 && operationIDs[0] != expectedOperation {
		return Result{}, fmt.Errorf("operations/order does not match %s phase %q", resultFormat, result.Phase)
	}
	if result.Status == "succeeded" && len(operationIDs) != 1 {
		return Result{}, fmt.Errorf("successful result is missing phase operation")
	}
	for _, id := range operationIDs {
		base := "operations/" + id + "/"
		status, err := r.text(base + "status")
		if err != nil {
			return Result{}, err
		}
		if err := requireOneOf(base+"status", status, "pending", "running", "succeeded", "failed", "cancelled", "unknown"); err != nil {
			return Result{}, err
		}
		detail, err := r.text(base + "detail")
		if err != nil {
			return Result{}, err
		}
		result.Operations = append(result.Operations, ResultOperation{ID: id, Status: status, Detail: detail})
	}
	if result.Status == "succeeded" && result.Operations[0].Status != "succeeded" {
		return Result{}, fmt.Errorf("successful result has non-successful operation")
	}
	if result.Status == "failed" || result.Status == "cancelled" {
		code, err := r.text("error/code")
		if err != nil {
			return Result{}, err
		}
		if !idPattern.MatchString(code) {
			return Result{}, fmt.Errorf("error/code contains invalid id %q", code)
		}
		detail, err := r.text("error/detail")
		if err != nil {
			return Result{}, err
		}
		operation, _, err := r.optionalText("error/operation")
		if err != nil {
			return Result{}, err
		}
		if operation != "" && operation != expectedOperation {
			return Result{}, fmt.Errorf("error/operation does not match phase %q", result.Phase)
		}
		result.Error = &ResultError{Code: code, Operation: operation, Detail: detail}
	}
	receiptPresent := false
	result.ReceiptPath, receiptPresent, err = r.optionalText("receipt/path")
	if err != nil {
		return Result{}, err
	}
	if result.Phase == "files" && result.Status == "succeeded" {
		if !receiptPresent || result.ReceiptPath == "" || !filepath.IsAbs(result.ReceiptPath) || filepath.Clean(result.ReceiptPath) != result.ReceiptPath {
			return Result{}, fmt.Errorf("successful files result has invalid receipt/path")
		}
	} else if receiptPresent {
		return Result{}, fmt.Errorf("receipt/path is invalid for %s phase status %s", result.Phase, result.Status)
	}
	return result, nil
}

func ReadApplyEvent(path string) (ApplyEvent, error) {
	r, err := openReader(path)
	if err != nil {
		return ApplyEvent{}, err
	}
	defer r.close()
	event := ApplyEvent{}
	for name, destination := range map[string]*string{
		"format":    &event.Format,
		"phase":     &event.Phase,
		"operation": &event.Operation,
		"status":    &event.Status,
		"detail":    &event.Detail,
	} {
		*destination, err = r.text(name)
		if err != nil {
			return ApplyEvent{}, err
		}
	}
	if event.Format != eventFormat {
		return ApplyEvent{}, fmt.Errorf("unsupported event format %q", event.Format)
	}
	if err := requireOneOf("phase", event.Phase, "checkout", "files"); err != nil {
		return ApplyEvent{}, err
	}
	if !idPattern.MatchString(event.Operation) {
		return ApplyEvent{}, fmt.Errorf("operation has invalid id %q", event.Operation)
	}
	expectedOperation := map[string]string{"checkout": "checkout-sync", "files": "write-files"}[event.Phase]
	if event.Operation != expectedOperation {
		return ApplyEvent{}, fmt.Errorf("phase %q event has operation %q, expected %q", event.Phase, event.Operation, expectedOperation)
	}
	if err := requireOneOf("status", event.Status, "started", "succeeded", "failed"); err != nil {
		return ApplyEvent{}, err
	}
	return event, nil
}
