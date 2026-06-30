package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type sszTarget struct {
	pkg, out string
	libInc   []string
	protoInc []string
	objs     []string
	exclude  []string
}

func genSSZ() error {
	targets, err := loadSSZTargets()
	if err != nil {
		return fmt.Errorf("load SSZ targets: %w", err)
	}

	minPb, err := os.MkdirTemp("", "gen-minpb-")
	if err != nil {
		return fmt.Errorf("mkdirTemp: %w", err)
	}
	defer func() { _ = os.RemoveAll(minPb) }()

	if err := emitMinimalPbgo(minPb); err != nil {
		return fmt.Errorf("emit minimal pb.go: %w", err)
	}

	for _, target := range targets {
		if err := genSSZTarget(target, minPb); err != nil {
			return fmt.Errorf("gen SSZ target: %w", err)
		}
	}

	return nil
}

func genSSZTarget(t sszTarget, minPb string) error {
	fmt.Printf("generating %s/%s\n", t.pkg, t.out)

	mainnet, err := sszgenOne(t, "")
	if err != nil {
		return fmt.Errorf("mainnet: %w", err)
	}

	minimal, err := sszgenOne(t, minPb)
	if err != nil {
		return fmt.Errorf("minimal: %w", err)
	}

	if mainnet == minimal {
		if err := os.WriteFile(filepath.Join(t.pkg, t.out), []byte(mainnet), 0o600); err != nil {
			return fmt.Errorf("writeFile: %w", err)
		}

		return nil
	}

	if err := os.WriteFile(filepath.Join(t.pkg, t.out), []byte("//go:build !minimal\n\n"+mainnet), 0o600); err != nil {
		return fmt.Errorf("writeFile: %w", err)
	}

	minOut := strings.TrimSuffix(t.out, ".ssz.go") + ".minimal.ssz.go"
	if err := os.WriteFile(filepath.Join(t.pkg, minOut), []byte("//go:build minimal\n\n"+minimal), 0o600); err != nil {
		return fmt.Errorf("writeFile: %w", err)
	}

	return nil
}

func sszgenOne(t sszTarget, root string) (string, error) {
	stage := filepath.Join(root, t.pkg, ".sszgen_tmp")
	if err := stagePbgo(filepath.Join(root, t.pkg), stage); err != nil {
		return "", fmt.Errorf("stagePbgo: %w", err)
	}

	defer unstage(stage)

	inc := slices.Clone(t.libInc)
	for _, p := range t.protoInc {
		istage := filepath.Join(root, p, ".sszinc_tmp")
		if err := stagePbgo(filepath.Join(root, p), istage); err != nil {
			return "", fmt.Errorf("stagePbgo: %w", err)
		}

		defer unstage(istage)
		inc = append(inc, istage)
	}

	tmp, err := os.CreateTemp("", "sszgen-*.go")
	if err != nil {
		return "", fmt.Errorf("createTemp: %w", err)
	}

	tmpName := tmp.Name()
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("close: %w", err)
	}

	defer func() { _ = os.Remove(tmpName) }()

	args := []string{"--output=" + tmpName, "--path=" + stage, "--objs=" + strings.Join(t.objs, ",")}
	if len(inc) > 0 {
		args = append(args, "--include="+strings.Join(inc, ","))
	}

	if len(t.exclude) > 0 {
		args = append(args, "--exclude-objs="+strings.Join(t.exclude, ","))
	}

	if err := sh("go", append([]string{"tool", "sszgen"}, args...)...); err != nil {
		return "", fmt.Errorf("sh: %w", err)
	}

	data, err := os.ReadFile(tmpName) // #nosec G304 -- tmpName is our own os.CreateTemp output
	if err != nil {
		return "", fmt.Errorf("readFile: %w", err)
	}

	var b strings.Builder
	for _, line := range strings.SplitAfter(string(data), "\n") {
		if strings.Contains(line, "// Hash: ") {
			continue
		}

		b.WriteString(line)
	}

	return b.String(), nil
}

func stagePbgo(pkgDir, stageDir string) error {
	if err := os.MkdirAll(stageDir, 0o750); err != nil {
		return fmt.Errorf("mkdirAll: %w", err)
	}

	entries, err := os.ReadDir(pkgDir)
	if err != nil {
		return fmt.Errorf("readDir: %w", err)
	}

	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".pb.go") || strings.HasSuffix(name, ".minimal.pb.go") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(pkgDir, name)) // #nosec G304 -- pkgDir/name from a controlled ReadDir of repo proto packages
		if err != nil {
			return fmt.Errorf("readFile: %w", err)
		}

		if err := os.WriteFile(filepath.Join(stageDir, name), data, 0o600); err != nil {
			return fmt.Errorf("writeFile: %w", err)
		}
	}

	return nil
}

func unstage(dir string) { _ = os.RemoveAll(dir) }
