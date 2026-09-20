package engine

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const BundledEngineRevision = "af663e2253e8d6dbdf9a36bd0a9fd374b0215fb7"

var bundledHashes = map[string]string{
	"public/sh/setup.sh":        "3fe433ed5b2fa9b3e12239b0bc4db75eea1bf4d8714220817c0575b3aa3875a2",
	"public/setup/profiles.tsv": "fff8d1c340fb1e87c76f80cac2224e28761ccc7e2b117839b7be6f5311a1ac11",
	"public/zsh/init.zsh":       "c979e39748d1d86ace17a61ff2b1bf6e1224a43291c25bf7fad985d1e9e11af1",
	"public/checksum.txt":       "4715c5a857d18f149a9578a1d7bbdacb3c18e3dc6ef5fdb75c5eb265a78d3503",
}

//go:embed bundled/public/sh/setup.sh bundled/public/setup/profiles.tsv bundled/public/zsh/init.zsh bundled/public/checksum.txt
var bundledFiles embed.FS

func extractBundledEngine(root string, inputs *Inputs) (string, error) {
	if err := verifyBundle(bundledFiles); err != nil {
		return "", err
	}
	engineRoot := filepath.Join(root, "bundled-engine")
	for name := range bundledHashes {
		data, err := fs.ReadFile(bundledFiles, "bundled/"+name)
		if err != nil {
			return "", fmt.Errorf("read bundled %s: %w", name, err)
		}
		destination := filepath.Join(engineRoot, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
			return "", fmt.Errorf("create bundled engine directory: %w", err)
		}
		mode := os.FileMode(0o600)
		if name == "public/sh/setup.sh" {
			mode = 0o700
		}
		if err := os.WriteFile(destination, data, mode); err != nil {
			return "", fmt.Errorf("write bundled %s: %w", name, err)
		}
	}
	inputs.Init = filepath.Join(engineRoot, "public", "zsh", "init.zsh")
	inputs.Profiles = filepath.Join(engineRoot, "public", "setup", "profiles.tsv")
	inputs.Checksum = filepath.Join(engineRoot, "public", "checksum.txt")
	return filepath.Join(engineRoot, "public", "sh", "setup.sh"), nil
}

func verifyBundle(bundle fs.FS) error {
	for name, expected := range bundledHashes {
		data, err := fs.ReadFile(bundle, "bundled/"+name)
		if err != nil {
			return fmt.Errorf("read bundled %s: %w", name, err)
		}
		digest := sha256.Sum256(data)
		actual := hex.EncodeToString(digest[:])
		if !strings.EqualFold(actual, expected) {
			return fmt.Errorf("bundled %s checksum mismatch: got %s, want %s", name, actual, expected)
		}
	}
	return nil
}
