// Package cdeps builds Prysm's C dependencies from source for the Go toolchain.
package cdeps

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// The hashtree module ships prebuilt C objects (hashtree_<os>_<arch>.syso). Hashtree compiles
// them from the module's sources instead, then points the build at a patched copy of the
// module through a `replace` in an alternate go.mod (-modfile). `-overlay` can't be used:
// cmd/go packs .syso files straight from the package directory.
const hashtreeModule = "github.com/OffchainLabs/hashtree"

// hashtreeSources lists the C/asm sources per GOARCH.
var hashtreeSources = map[string][]string{
	"amd64": {
		"hashtree.c", "sha256_generic.c",
		"sha256_avx_x1.S", "sha256_avx_x4.S", "sha256_avx_x8.S", "sha256_avx_x16.S",
		"sha256_shani.S", "sha256_sse_x1.S",
	},
	"arm64": {
		"hashtree.c", "sha256_generic.c",
		"sha256_armv8_crypto.S", "sha256_armv8_neon_x1.S", "sha256_armv8_neon_x4.S",
	},
}

// Hashtree is a from-source copy of the hashtree module under Dir.
//
// Dir must be relative to the repo root: the replace target is recorded in the binaries'
// build info (even with -trimpath), so an absolute path would make the output depend on the
// checkout location.
type Hashtree struct {
	Dir string
}

// Prepare copies the hashtree module to Dir without its prebuilt .syso files and writes the
// alternate go.mod replacing the module with that copy. It returns the -modfile path.
func (h Hashtree) Prepare(goBin string) (string, error) {
	// #nosec G204 -- build-time tooling: goBin comes from the Makefile environment.
	out, err := exec.Command(goBin, "mod", "download", "-json", hashtreeModule).Output()
	if err != nil {
		return "", fmt.Errorf("go mod download %s: %w", hashtreeModule, err)
	}

	var mod struct{ Dir string }
	if err := json.Unmarshal(out, &mod); err != nil {
		return "", fmt.Errorf("parse go mod download output: %w", err)
	}

	dst := h.moduleDir()
	if err := os.RemoveAll(h.Dir); err != nil {
		return "", fmt.Errorf("clean %s: %w", h.Dir, err)
	}

	if err := os.CopyFS(dst, os.DirFS(mod.Dir)); err != nil {
		return "", fmt.Errorf("copy %s: %w", hashtreeModule, err)
	}

	prebuilt, err := filepath.Glob(filepath.Join(dst, "*.syso"))
	if err != nil {
		return "", fmt.Errorf("glob: %w", err)
	}

	for _, f := range prebuilt {
		if err := os.Remove(f); err != nil {
			return "", fmt.Errorf("remove prebuilt: %w", err)
		}
	}

	for _, name := range []string{"go.mod", "go.sum"} {
		b, err := os.ReadFile(name) // #nosec G304 -- fixed in-repo file names.
		if err != nil {
			return "", fmt.Errorf("read %s: %w", name, err)
		}

		if err := os.WriteFile(filepath.Join(h.Dir, name), b, 0o600); err != nil {
			return "", fmt.Errorf("write %s: %w", name, err)
		}
	}

	modfile := filepath.Join(h.Dir, "go.mod")

	// #nosec G204 -- build-time tooling: goBin comes from the Makefile environment.
	edit := exec.Command(goBin, "mod", "edit", "-modfile="+modfile, "-replace="+hashtreeModule+"=./"+dst)
	edit.Stderr = os.Stderr
	if err := edit.Run(); err != nil {
		return "", fmt.Errorf("go mod edit: %w", err)
	}

	return modfile, nil
}

// Supported reports whether hashtree has native code for goos/goarch. Elsewhere the module
// falls back to pure Go and no .syso is needed.
func Supported(goos, goarch string) bool {
	_, ok := hashtreeSources[goarch]
	return ok && (goos != "darwin" || goarch != "amd64")
}

// Build compiles hashtree_<goos>_<goarch>.syso for one target with the target's C compiler
// (cc, possibly with arguments), with -O3 -Wall (plus -fno-integrated-as on non-Windows
// amd64). pathPrefix is prepended to PATH for the compiler, zig archives the objects. It is a
// no-op for targets without native code.
func (h Hashtree) Build(goos, goarch, cc, pathPrefix, zig string) error {
	if !Supported(goos, goarch) {
		return nil
	}

	flags := []string{"-O3", "-Wall", "-g0"}
	if goarch == "amd64" && goos != "windows" {
		flags = append(flags, "-fno-integrated-as")
	}

	srcDir := filepath.Join(h.moduleDir(), "src")
	objDir := filepath.Join(h.Dir, "obj", goos+"_"+goarch)
	if err := os.MkdirAll(objDir, 0o750); err != nil {
		return fmt.Errorf("create obj dir: %w", err)
	}

	absObjDir, err := filepath.Abs(objDir)
	if err != nil {
		return fmt.Errorf("abs: %w", err)
	}

	ccArgs := strings.Fields(cc)
	srcs := hashtreeSources[goarch]
	objs := make([]string, 0, len(srcs))
	for _, src := range srcs {
		obj := filepath.Join(absObjDir, strings.TrimSuffix(src, filepath.Ext(src))+".o")
		args := slices.Concat(ccArgs[1:], flags, []string{"-c", src, "-o", obj})

		// #nosec G204 -- build-time tooling: the compiler comes from the toolchain selection.
		cmd := exec.Command(ccArgs[0], args...)
		cmd.Dir = srcDir
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if pathPrefix != "" {
			cmd.Env = append(os.Environ(), "PATH="+pathPrefix+os.Getenv("PATH"))
		}

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("compile %s: %w", src, err)
		}

		objs = append(objs, obj)
	}

	format := "gnu"
	if goos == "darwin" {
		format = "darwin"
	}

	syso := filepath.Join(h.moduleDir(), fmt.Sprintf("hashtree_%s_%s.syso", goos, goarch))
	// #nosec G204 -- build-time tooling: zig is provisioned by install-zig.sh.
	ar := exec.Command(zig, append([]string{"ar", "--format=" + format, "rcsD", syso}, objs...)...)
	ar.Stdout, ar.Stderr = os.Stdout, os.Stderr
	if err := ar.Run(); err != nil {
		return fmt.Errorf("archive %s: %w", syso, err)
	}

	return nil
}

func (h Hashtree) moduleDir() string {
	return filepath.Join(h.Dir, "hashtree")
}

// Provision runs one of the tools/cross-toolchain install scripts and returns the path it
// prints.
func Provision(script string) (string, error) {
	// #nosec G204 -- build-time tooling: `script` is one of the in-repo literals passed by callers.
	cmd := exec.Command(filepath.Join("tools", "cross-toolchain", script))
	cmd.Stderr = os.Stderr

	path, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%s: %w", script, err)
	}

	return strings.TrimSpace(string(path)), nil
}
