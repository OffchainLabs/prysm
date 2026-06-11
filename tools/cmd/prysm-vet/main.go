// Command prysm-vet is Prysm's static-analysis driver, replacing Bazel's `nogo`
// (Phase 7 of the Bazel->Go-toolchain migration). It is a standard
// golang.org/x/tools multichecker that embeds the exact analyzer set nogo ran —
// the custom analyzers under tools/analyzers/, the enabled golang.org/x/tools
// passes, and the staticcheck SA* checks (minus SA1019) — and reproduces nogo's
// per-analyzer file exclusions by reading nogo_config.json and filtering each
// analyzer's diagnostics by file path.
//
// Run via `make lint` (which invokes `go run ./tools/cmd/prysm-vet ./...` from the
// repo root). The config path defaults to ./nogo_config.json and can be overridden
// with PRYSM_VET_CONFIG. `prysm-vet help` lists the registered analyzers.
package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/multichecker"

	// golang.org/x/tools passes (the set nogo enabled in BUILD.bazel; cgocall,
	// fieldalignment and shadow are intentionally omitted — disabled there). pkgfact
	// is also omitted: it is x/tools' fact-mechanism *demonstration* analyzer (not a
	// real check) and only emits noise like `name="value"`; nogo listing it was incidental.
	"golang.org/x/tools/go/analysis/passes/appends"
	"golang.org/x/tools/go/analysis/passes/asmdecl"
	"golang.org/x/tools/go/analysis/passes/assign"
	"golang.org/x/tools/go/analysis/passes/atomic"
	"golang.org/x/tools/go/analysis/passes/atomicalign"
	"golang.org/x/tools/go/analysis/passes/bools"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/analysis/passes/buildtag"
	"golang.org/x/tools/go/analysis/passes/composite"
	"golang.org/x/tools/go/analysis/passes/copylock"
	"golang.org/x/tools/go/analysis/passes/ctrlflow"
	"golang.org/x/tools/go/analysis/passes/deepequalerrors"
	"golang.org/x/tools/go/analysis/passes/defers"
	"golang.org/x/tools/go/analysis/passes/directive"
	"golang.org/x/tools/go/analysis/passes/errorsas"
	"golang.org/x/tools/go/analysis/passes/findcall"
	"golang.org/x/tools/go/analysis/passes/framepointer"
	"golang.org/x/tools/go/analysis/passes/httpmux"
	"golang.org/x/tools/go/analysis/passes/httpresponse"
	"golang.org/x/tools/go/analysis/passes/ifaceassert"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/analysis/passes/loopclosure"
	"golang.org/x/tools/go/analysis/passes/lostcancel"
	"golang.org/x/tools/go/analysis/passes/nilfunc"
	"golang.org/x/tools/go/analysis/passes/nilness"
	"golang.org/x/tools/go/analysis/passes/printf"
	"golang.org/x/tools/go/analysis/passes/reflectvaluecompare"
	"golang.org/x/tools/go/analysis/passes/shift"
	"golang.org/x/tools/go/analysis/passes/sigchanyzer"
	"golang.org/x/tools/go/analysis/passes/slog"
	"golang.org/x/tools/go/analysis/passes/sortslice"
	"golang.org/x/tools/go/analysis/passes/stdmethods"
	"golang.org/x/tools/go/analysis/passes/stringintconv"
	"golang.org/x/tools/go/analysis/passes/structtag"
	"golang.org/x/tools/go/analysis/passes/testinggoroutine"
	"golang.org/x/tools/go/analysis/passes/tests"
	"golang.org/x/tools/go/analysis/passes/timeformat"
	"golang.org/x/tools/go/analysis/passes/unmarshal"
	"golang.org/x/tools/go/analysis/passes/unreachable"
	"golang.org/x/tools/go/analysis/passes/unsafeptr"
	"golang.org/x/tools/go/analysis/passes/unusedresult"
	"golang.org/x/tools/go/analysis/passes/unusedwrite"
	"golang.org/x/tools/go/analysis/passes/usesgenerics"

	"honnef.co/go/tools/staticcheck"

	// Custom Prysm analyzers (tools/analyzers/*).
	"github.com/OffchainLabs/prysm/v7/tools/analyzers/comparesame"
	"github.com/OffchainLabs/prysm/v7/tools/analyzers/cryptorand"
	"github.com/OffchainLabs/prysm/v7/tools/analyzers/errcheck"
	"github.com/OffchainLabs/prysm/v7/tools/analyzers/featureconfig"
	"github.com/OffchainLabs/prysm/v7/tools/analyzers/gocognit"
	"github.com/OffchainLabs/prysm/v7/tools/analyzers/httpwriter"
	"github.com/OffchainLabs/prysm/v7/tools/analyzers/ineffassign"
	"github.com/OffchainLabs/prysm/v7/tools/analyzers/interfacechecker"
	"github.com/OffchainLabs/prysm/v7/tools/analyzers/logcapitalization"
	"github.com/OffchainLabs/prysm/v7/tools/analyzers/logruswitherror"
	"github.com/OffchainLabs/prysm/v7/tools/analyzers/maligned"
	mzany "github.com/OffchainLabs/prysm/v7/tools/analyzers/modernize/any"
	mzappendclipped "github.com/OffchainLabs/prysm/v7/tools/analyzers/modernize/appendclipped"
	mzbloop "github.com/OffchainLabs/prysm/v7/tools/analyzers/modernize/bloop"
	mzfmtappendf "github.com/OffchainLabs/prysm/v7/tools/analyzers/modernize/fmtappendf"
	mzforvar "github.com/OffchainLabs/prysm/v7/tools/analyzers/modernize/forvar"
	mzmapsloop "github.com/OffchainLabs/prysm/v7/tools/analyzers/modernize/mapsloop"
	mzminmax "github.com/OffchainLabs/prysm/v7/tools/analyzers/modernize/minmax"
	mzomitzero "github.com/OffchainLabs/prysm/v7/tools/analyzers/modernize/omitzero"
	mzrangeint "github.com/OffchainLabs/prysm/v7/tools/analyzers/modernize/rangeint"
	mzreflecttypefor "github.com/OffchainLabs/prysm/v7/tools/analyzers/modernize/reflecttypefor"
	mzslicescontains "github.com/OffchainLabs/prysm/v7/tools/analyzers/modernize/slicescontains"
	mzslicessort "github.com/OffchainLabs/prysm/v7/tools/analyzers/modernize/slicessort"
	mzstringsbuilder "github.com/OffchainLabs/prysm/v7/tools/analyzers/modernize/stringsbuilder"
	mzstringscutprefix "github.com/OffchainLabs/prysm/v7/tools/analyzers/modernize/stringscutprefix"
	mzstringsseq "github.com/OffchainLabs/prysm/v7/tools/analyzers/modernize/stringsseq"
	mztestingcontext "github.com/OffchainLabs/prysm/v7/tools/analyzers/modernize/testingcontext"
	mzwaitgroup "github.com/OffchainLabs/prysm/v7/tools/analyzers/modernize/waitgroup"
	"github.com/OffchainLabs/prysm/v7/tools/analyzers/nop"
	"github.com/OffchainLabs/prysm/v7/tools/analyzers/nopanic"
	"github.com/OffchainLabs/prysm/v7/tools/analyzers/properpermissions"
	"github.com/OffchainLabs/prysm/v7/tools/analyzers/recursivelock"
	"github.com/OffchainLabs/prysm/v7/tools/analyzers/shadowpredecl"
	"github.com/OffchainLabs/prysm/v7/tools/analyzers/slicedirect"
	"github.com/OffchainLabs/prysm/v7/tools/analyzers/uintcast"
)

// disabledStaticcheck lists the staticcheck SA* checks NOT to register, mirroring
// the commented-out entries in BUILD.bazel's STATICCHECK_ANALYZERS list.
var disabledStaticcheck = map[string]bool{
	"SA1019": true, // deprecated-symbol use; nogo had it off (TODO: fix all uses).
}

func main() {
	cfgPath := os.Getenv("PRYSM_VET_CONFIG")
	if cfgPath == "" {
		cfgPath = "nogo_config.json"
	}
	cfg, err := loadConfig(cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "❌ prysm-vet:", err)
		os.Exit(1)
	}

	analyzers := registry()

	// Wrap every analyzer so its diagnostics are filtered by (a) nogo_config.json's
	// per-analyzer only_files/exclude_files and (b) a baseline that keeps analysis
	// first-party — dropping anything under the module cache or GOROOT (the Go-module
	// equivalent of nogo's "external/.*" exclusions).
	drop := dependencyPrefixes()
	for _, a := range analyzers {
		withFilter(a, cfg[a.Name], drop)
	}

	multichecker.Main(analyzers...)
}

// registry returns the full set of analyzers to run, matching BUILD.bazel:175-273.
func registry() []*analysis.Analyzer {
	analyzers := []*analysis.Analyzer{
		// Custom analyzers.
		comparesame.Analyzer, cryptorand.Analyzer, errcheck.Analyzer, featureconfig.Analyzer,
		gocognit.Analyzer, httpwriter.Analyzer, ineffassign.Analyzer, interfacechecker.Analyzer,
		logcapitalization.Analyzer, logruswitherror.Analyzer, maligned.Analyzer, nop.Analyzer,
		nopanic.Analyzer, properpermissions.Analyzer, recursivelock.Analyzer, shadowpredecl.Analyzer,
		slicedirect.Analyzer, uintcast.Analyzer,
		// modernize/* wrappers (newexpr and slicesdelete are disabled in BUILD.bazel).
		mzany.Analyzer, mzappendclipped.Analyzer, mzbloop.Analyzer, mzfmtappendf.Analyzer,
		mzforvar.Analyzer, mzmapsloop.Analyzer, mzminmax.Analyzer, mzomitzero.Analyzer,
		mzrangeint.Analyzer, mzreflecttypefor.Analyzer, mzslicescontains.Analyzer, mzslicessort.Analyzer,
		mzstringsbuilder.Analyzer, mzstringscutprefix.Analyzer, mzstringsseq.Analyzer,
		mztestingcontext.Analyzer, mzwaitgroup.Analyzer,
		// golang.org/x/tools passes.
		appends.Analyzer, asmdecl.Analyzer, assign.Analyzer, atomic.Analyzer, atomicalign.Analyzer,
		bools.Analyzer, buildssa.Analyzer, buildtag.Analyzer, composite.Analyzer, copylock.Analyzer,
		ctrlflow.Analyzer, deepequalerrors.Analyzer, defers.Analyzer, directive.Analyzer,
		errorsas.Analyzer, findcall.Analyzer, framepointer.Analyzer, httpmux.Analyzer,
		httpresponse.Analyzer, ifaceassert.Analyzer, inspect.Analyzer, loopclosure.Analyzer,
		lostcancel.Analyzer, nilfunc.Analyzer, nilness.Analyzer, printf.Analyzer,
		reflectvaluecompare.Analyzer, shift.Analyzer, sigchanyzer.Analyzer, slog.Analyzer,
		sortslice.Analyzer, stdmethods.Analyzer, stringintconv.Analyzer, structtag.Analyzer,
		testinggoroutine.Analyzer, tests.Analyzer, timeformat.Analyzer, unmarshal.Analyzer,
		unreachable.Analyzer, unsafeptr.Analyzer, unusedresult.Analyzer, unusedwrite.Analyzer,
		usesgenerics.Analyzer,
	}
	// staticcheck SA* checks, minus the disabled ones.
	for _, a := range staticcheck.Analyzers {
		if disabledStaticcheck[a.Analyzer.Name] {
			continue
		}
		analyzers = append(analyzers, a.Analyzer)
	}
	return analyzers
}

// filter is the compiled only_files/exclude_files set for one analyzer.
type filter struct {
	only    []*regexp.Regexp
	exclude []*regexp.Regexp
}

// allows reports whether a diagnostic in file should be kept: exclude_files wins,
// then only_files restricts (if present). Matches nogo's precedence.
func (f *filter) allows(file string) bool {
	if f == nil {
		return true
	}
	for _, re := range f.exclude {
		if re.MatchString(file) {
			return false
		}
	}
	if len(f.only) > 0 {
		for _, re := range f.only {
			if re.MatchString(file) {
				return true
			}
		}
		return false
	}
	return true
}

// withFilter rewrites a.Run so its reported diagnostics are passed through, in order,
// the generated-file skip, the baseline dependency-path drop, and the analyzer's nogo
// config filter.
func withFilter(a *analysis.Analyzer, flt *filter, drop []string) {
	orig := a.Run
	a.Run = func(pass *analysis.Pass) (any, error) {
		// Skip generated files for every analyzer. Bazel's nogo effectively never
		// linted generated code (its `.pb.go`/`.ssz.go` exclusions for ineffassign/
		// gocognit show the intent); modernize & friends don't self-skip, so do it
		// centrally here via the standard `// Code generated ... DO NOT EDIT.` marker.
		generated := make(map[string]bool)
		for _, af := range pass.Files {
			if ast.IsGenerated(af) {
				if tf := pass.Fset.File(af.Pos()); tf != nil {
					generated[tf.Name()] = true
				}
			}
		}
		report := pass.Report
		pass.Report = func(d analysis.Diagnostic) {
			file := ""
			if tf := pass.Fset.File(d.Pos); tf != nil {
				file = tf.Name()
			}
			if file != "" {
				if generated[file] {
					return
				}
				for _, p := range drop {
					if strings.HasPrefix(file, p) {
						return
					}
				}
				if !flt.allows(file) {
					return
				}
			}
			report(d)
		}
		return orig(pass)
	}
}

// loadConfig parses nogo_config.json into a per-analyzer filter map keyed by
// analyzer Name (the same keys nogo used).
func loadConfig(path string) (map[string]*filter, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading vet config %q: %w", path, err)
	}
	var parsed map[string]struct {
		OnlyFiles    map[string]string `json:"only_files"`
		ExcludeFiles map[string]string `json:"exclude_files"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("parsing vet config %q: %w", path, err)
	}
	out := make(map[string]*filter, len(parsed))
	for name, entry := range parsed {
		f := &filter{}
		var perr error
		if f.only, perr = compile(entry.OnlyFiles); perr != nil {
			return nil, fmt.Errorf("%s.only_files: %w", name, perr)
		}
		if f.exclude, perr = compile(entry.ExcludeFiles); perr != nil {
			return nil, fmt.Errorf("%s.exclude_files: %w", name, perr)
		}
		out[name] = f
	}
	return out, nil
}

func compile(patterns map[string]string) ([]*regexp.Regexp, error) {
	res := make([]*regexp.Regexp, 0, len(patterns))
	for p := range patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, fmt.Errorf("bad pattern %q: %w", p, err)
		}
		res = append(res, re)
	}
	return res, nil
}

// dependencyPrefixes returns the file-path prefixes whose diagnostics are always
// dropped (module cache, GOROOT, build cache), keeping analysis first-party. This is
// the Go-module analogue of nogo's pervasive "external/.*" / "rules_go_work-.*"
// exclusions, which never match in-tree paths here. GOCACHE catches positions that
// resolve into cgo/compiled artifacts under the build cache.
func dependencyPrefixes() []string {
	var prefixes []string
	for _, v := range []string{"GOMODCACHE", "GOROOT", "GOCACHE"} {
		if out, err := exec.Command("go", "env", v).Output(); err == nil {
			if p := strings.TrimSpace(string(out)); p != "" {
				prefixes = append(prefixes, p)
			}
		}
	}
	return prefixes
}
