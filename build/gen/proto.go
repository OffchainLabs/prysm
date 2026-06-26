package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bufbuild/protocompile"
	"github.com/bufbuild/protocompile/protoutil"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

const (
	castPin       = "v0.0.0-20230228205207-28762a7b9294"
	protobufGoVer = "v1.36.3"

	googleapisPin    = "64926d52febbf298cb82a8f472ade4a3969ba922"
	googleapisSHA256 = "9d1a930e767c93c825398b8f8692eca3fe353b9aaadedfbcf1fca2282c85df88"
)

var googleapisProtos = []string{
	"google/api/annotations.proto",
	"google/api/http.proto",
}

const (
	modeCast     = "cast"
	modeCastGRPC = "cast_grpc"
	modeStock    = "stock"
)

// typeDiffering lists the generated *.pb.go whose Go
// field TYPE differs between mainnet and minimal.
var typeDiffering = map[string]bool{
	"proto/prysm/v1alpha1/attestation.pb.go":       true,
	"proto/prysm/v1alpha1/sync_committee.pb.go":    true,
	"proto/prysm/v1alpha1/beacon_core_types.pb.go": true,
	"proto/prysm/v1alpha1/gloas.pb.go":             true,
	"proto/eth/v1/beacon_block.pb.go":              true,
}

func protoPkgDirs(pkgs map[string]string) []string {
	dirs := make([]string, 0, len(pkgs))
	for dir := range pkgs {
		dirs = append(dirs, dir)
	}

	sort.Strings(dirs)

	return dirs
}

func genProto() error {
	mainnet, minimal, err := loadSSZDicts()
	if err != nil {
		return fmt.Errorf("load SSZ dicts: %w", err)
	}

	pkgs, err := loadProtoPkgs()
	if err != nil {
		return fmt.Errorf("load proto pkgs: %w", err)
	}

	tmpRoot, err := os.MkdirTemp("", "gen-proto-")
	if err != nil {
		return fmt.Errorf("mkdirTemp: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpRoot) }()

	googleapisInc, err := fetchGoogleapis(filepath.Join(tmpRoot, "googleapis"))
	if err != nil {
		return fmt.Errorf("fetch googleapis: %w", err)
	}

	binDir, err := buildProtoPlugins(tmpRoot)
	if err != nil {
		return fmt.Errorf("build proto plugins: %w", err)
	}

	outMain := filepath.Join(tmpRoot, "mainnet")
	if err := generateNetwork(mainnet, outMain, binDir, googleapisInc, pkgs); err != nil {
		return fmt.Errorf("generate mainnet: %w", err)
	}

	outMin := filepath.Join(tmpRoot, "minimal")
	if err := generateNetwork(minimal, outMin, binDir, googleapisInc, pkgs); err != nil {
		return fmt.Errorf("generate minimal: %w", err)
	}

	// Write each mainnet *.pb.go back to its source-relative path:
	// - untagged for config-invariant protos
	// - //go:build !minimal file plus a <name>.minimal.pb.go twin for the type-differing ones.
	err = filepath.WalkDir(outMain, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".pb.go") {
			return err
		}

		rel, err := filepath.Rel(outMain, path)
		if err != nil {
			return fmt.Errorf("filepath.Rel: %w", err)
		}
		rel = filepath.ToSlash(rel)

		if !typeDiffering[rel] {
			return copyFile(path, rel)
		}

		if err := writeTagged("!minimal", path, rel); err != nil {
			return fmt.Errorf("writeTagged: %w", err)
		}

		minTwin := strings.TrimSuffix(rel, ".pb.go") + ".minimal.pb.go"

		if err := writeTagged("minimal", filepath.Join(outMin, rel), minTwin); err != nil {
			return fmt.Errorf("writeTagged: %w", err)
		}

		return nil
	})
	if err != nil {
		return err
	}

	if err := goimports("proto"); err != nil {
		return fmt.Errorf("goimports: %w", err)
	}

	if err := gofmtSimplify("proto"); err != nil {
		return fmt.Errorf("gofmtSimplify: %w", err)
	}

	if err := applyGenModes("proto"); err != nil {
		return fmt.Errorf("applyGenModes: %w", err)
	}

	return nil
}

// The 0755 mode mimics the old Bazel codegen, which marked its outputs
// executable. That makes no sense for Go source files, but we reproduce it
// here to avoid a large mode diff across the whole proto tree.
func applyGenModes(root string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".pb.go") {
			return err
		}

		mode := os.FileMode(0o755)
		if strings.HasSuffix(path, ".minimal.pb.go") {
			mode = 0o644
		}

		if err := os.Chmod(path, mode); err != nil {
			return fmt.Errorf("chmod %s: %w", path, err)
		}

		return nil
	})
}

func emitMinimalPbgo(dir string) error {
	_, minimal, err := loadSSZDicts()
	if err != nil {
		return fmt.Errorf("load SSZ dicts: %w", err)
	}

	pkgs, err := loadProtoPkgs()
	if err != nil {
		return fmt.Errorf("load proto pkgs: %w", err)
	}

	tmpRoot, err := os.MkdirTemp("", "gen-minpb-")
	if err != nil {
		return fmt.Errorf("mkdirTemp: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpRoot) }()

	googleapisInc, err := fetchGoogleapis(filepath.Join(tmpRoot, "googleapis"))
	if err != nil {
		return fmt.Errorf("fetch googleapis: %w", err)
	}

	binDir, err := buildProtoPlugins(tmpRoot)
	if err != nil {
		return fmt.Errorf("build proto plugins: %w", err)
	}

	return generateNetwork(minimal, dir, binDir, googleapisInc, pkgs)
}

func fetchGoogleapis(dest string) (string, error) {
	url := fmt.Sprintf("https://github.com/googleapis/googleapis/archive/%s.zip", googleapisPin)

	data, err := downloadVerified(url, googleapisSHA256)
	if err != nil {
		return "", fmt.Errorf("download googleapis archive: %w", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("open zip: %w", err)
	}

	stripPrefix := fmt.Sprintf("googleapis-%s/", googleapisPin)

	want := make(map[string]bool, len(googleapisProtos))
	for _, p := range googleapisProtos {
		want[p] = true
	}

	found := make(map[string]bool, len(want))
	for _, f := range zr.File {
		rel := strings.TrimPrefix(f.Name, stripPrefix)
		if !want[rel] {
			continue
		}

		if err := extractZipFile(f, dest, rel); err != nil {
			return "", fmt.Errorf("extract %s: %w", rel, err)
		}

		found[rel] = true
	}

	for _, p := range googleapisProtos {
		if !found[p] {
			return "", fmt.Errorf("googleapis archive missing %s", p)
		}
	}

	return dest, nil
}

func downloadVerified(url, wantSHA256 string) ([]byte, error) {
	resp, err := http.Get(url) // #nosec G107 -- url is built from the pinned googleapis commit constant
	if err != nil {
		return nil, fmt.Errorf("http get: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http get %s: %s", url, resp.Status)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("readAll: %w", err)
	}

	if sum := fmt.Sprintf("%x", sha256.Sum256(data)); sum != wantSHA256 {
		return nil, fmt.Errorf("sha256 mismatch for %s: got %s, want %s", url, sum, wantSHA256)
	}

	return data, nil
}

func extractZipFile(f *zip.File, destDir, rel string) error {
	dst := filepath.Join(destDir, filepath.FromSlash(rel))
	if !strings.HasPrefix(dst, filepath.Clean(destDir)+string(os.PathSeparator)) {
		return fmt.Errorf("archive entry %q escapes destination directory", rel)
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return fmt.Errorf("mkdirAll: %w", err)
	}

	rc, err := f.Open()
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer func() { _ = rc.Close() }()

	data, err := io.ReadAll(rc) // #nosec G110 -- archive is SHA-256 verified above
	if err != nil {
		return fmt.Errorf("readAll: %w", err)
	}

	return os.WriteFile(dst, data, 0o600)
}

func generateNetwork(dict map[string]string, outDir, binDir, googleapisInc string, pkgs map[string]string) error {
	tmp, err := os.MkdirTemp("", "gen-net-")
	if err != nil {
		return fmt.Errorf("mkdirTemp: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	stage := filepath.Join(tmp, "stage")
	if err := stageProtos(stage, dict); err != nil {
		return fmt.Errorf("stageProtos: %w", err)
	}

	mmap, err := buildMMAP(stage)
	if err != nil {
		return fmt.Errorf("buildMMAP: %w", err)
	}

	var allProtos []string
	for _, dir := range protoPkgDirs(pkgs) {
		protos, err := pkgProtos(dir)
		if err != nil {
			return fmt.Errorf("pkgProtos: %w", err)
		}

		allProtos = append(allProtos, protos...)
	}

	protoFiles, err := compileDescriptors(stage, googleapisInc, allProtos)
	if err != nil {
		return fmt.Errorf("compileDescriptors: %w", err)
	}

	if err := os.MkdirAll(outDir, 0o750); err != nil {
		return fmt.Errorf("mkdir all: %w", err)
	}

	for _, dir := range protoPkgDirs(pkgs) {
		mode := pkgs[dir]
		protos, err := pkgProtos(dir)
		if err != nil {
			return fmt.Errorf("pkgProtos: %w", err)
		}

		fmt.Printf("  %s (%s): %d files\n", dir, mode, len(protos))
		opt := "paths=source_relative" + mmap
		plugin, param, err := pluginForMode(mode, binDir, opt)
		if err != nil {
			return fmt.Errorf("pluginForMode: %w", err)
		}

		req := &pluginpb.CodeGeneratorRequest{
			FileToGenerate: protos,
			Parameter:      &param,
			ProtoFile:      protoFiles,
		}

		if err := runPlugin(plugin, req, outDir); err != nil {
			return fmt.Errorf("runPlugin %s: %w", filepath.Base(plugin), err)
		}
	}

	return nil
}

func compileDescriptors(stage, googleapisInc string, files []string) ([]*descriptorpb.FileDescriptorProto, error) {
	compiler := protocompile.Compiler{
		Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{
			ImportPaths: []string{stage, googleapisInc},
		}),
		SourceInfoMode: protocompile.SourceInfoNone,
	}

	linked, err := compiler.Compile(context.Background(), files...)
	if err != nil {
		return nil, fmt.Errorf("compile: %w", err)
	}

	var (
		out   []*descriptorpb.FileDescriptorProto
		visit func(fd protoreflect.FileDescriptor)
	)

	seen := make(map[string]bool)
	visit = func(fd protoreflect.FileDescriptor) {
		if seen[fd.Path()] {
			return
		}

		seen[fd.Path()] = true

		imports := fd.Imports()
		for i := range imports.Len() {
			visit(imports.Get(i).FileDescriptor)
		}

		fdp := protoutil.ProtoFromFileDescriptor(fd)
		fdp.SourceCodeInfo = nil
		out = append(out, fdp)
	}

	for _, f := range linked {
		visit(f)
	}

	return out, nil
}

func runPlugin(plugin string, req *pluginpb.CodeGeneratorRequest, outDir string) error {
	in, err := proto.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	cmd := exec.Command(plugin) // #nosec G204 -- plugin is a path under our own temp bin dir
	cmd.Stdin = bytes.NewReader(in)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: %s", err, stderr.String())
	}

	var resp pluginpb.CodeGeneratorResponse
	if err := proto.Unmarshal(stdout.Bytes(), &resp); err != nil {
		return fmt.Errorf("unmarshal response: %w", err)
	}

	if msg := resp.GetError(); msg != "" {
		return fmt.Errorf("plugin reported error: %s", msg)
	}

	for _, f := range resp.GetFile() {
		dst := filepath.Join(outDir, filepath.FromSlash(f.GetName()))
		if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
			return fmt.Errorf("mkdir all: %w", err)
		}

		if err := os.WriteFile(dst, []byte(f.GetContent()), 0o600); err != nil {
			return fmt.Errorf("writeFile: %w", err)
		}
	}

	return nil
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src) // #nosec G304 -- src is under our own temp out dir
	if err != nil {
		return fmt.Errorf("readFile: %w", err)
	}

	return os.WriteFile(dst, data, 0o600)
}

func writeTagged(tag, src, dst string) error {
	data, err := os.ReadFile(src) // #nosec G304 -- src is under our own temp out dir
	if err != nil {
		return fmt.Errorf("readFile: %w", err)
	}

	return os.WriteFile(dst, []byte("//go:build "+tag+"\n\n"+string(data)), 0o600)
}

func pluginForMode(mode, binDir, baseOpt string) (plugin, param string, err error) {
	castPlugin := filepath.Join(binDir, "protoc-gen-go-cast")
	switch mode {
	case modeCast:
		return castPlugin, baseOpt, nil
	case modeCastGRPC:
		return castPlugin, baseOpt + ",plugins=grpc", nil
	case modeStock:
		return filepath.Join(binDir, "protoc-gen-go"), baseOpt, nil
	default:
		return "", "", fmt.Errorf("unknown proto plugin mode %q", mode)
	}
}

func buildProtoPlugins(tmpRoot string) (string, error) {
	binDir := filepath.Join(tmpRoot, "bin")
	if err := os.MkdirAll(binDir, 0o750); err != nil {
		return "", fmt.Errorf("mkdir all: %w", err)
	}

	pluginMod := filepath.Join(tmpRoot, "pluginmod")
	if err := os.MkdirAll(pluginMod, 0o750); err != nil {
		return "", fmt.Errorf("mkdir all: %w", err)
	}

	gomod := fmt.Sprintf("module pluginbuild\ngo 1.23\nrequire github.com/prysmaticlabs/protoc-gen-go-cast %s\nrequire google.golang.org/protobuf %s\n", castPin, protobufGoVer)
	if err := os.WriteFile(filepath.Join(pluginMod, "go.mod"), []byte(gomod), 0o600); err != nil {
		return "", fmt.Errorf("writeFile: %w", err)
	}

	fmt.Printf("building protoc-gen-go-cast + protoc-gen-go against protobuf-go %s\n", protobufGoVer)
	env := []string{"GOFLAGS=-mod=mod"}
	if err := shInDir(pluginMod, env, "go", "build", "-o", filepath.Join(binDir, "protoc-gen-go-cast"), "github.com/prysmaticlabs/protoc-gen-go-cast"); err != nil {
		return "", fmt.Errorf("shInDir: %w", err)
	}

	if err := shInDir(pluginMod, env, "go", "build", "-o", filepath.Join(binDir, "protoc-gen-go"), "google.golang.org/protobuf/cmd/protoc-gen-go"); err != nil {
		return "", fmt.Errorf("shInDir: %w", err)
	}

	return binDir, nil
}

func stageProtos(stage string, dict map[string]string) error {
	keys := make([]string, 0, len(dict))
	for k := range dict {
		keys = append(keys, k)
	}

	sort.Slice(keys, func(i, j int) bool {
		if len(keys[i]) != len(keys[j]) {
			return len(keys[i]) > len(keys[j])
		}

		return keys[i] < keys[j]
	})

	return filepath.WalkDir("proto", func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".proto") {
			return err
		}
		data, err := os.ReadFile(path) // #nosec G304 -- path comes from WalkDir over the repo proto tree
		if err != nil {
			return fmt.Errorf("readFile: %w", err)
		}

		s := string(data)
		for _, k := range keys {
			s = strings.ReplaceAll(s, k, dict[k])
		}

		dst := filepath.Join(stage, path)
		if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
			return fmt.Errorf("mkdir all: %w", err)
		}

		if err := os.WriteFile(dst, []byte(s), 0o600); err != nil {
			return fmt.Errorf("writeFile: %w", err)
		}

		return nil
	})
}

func buildMMAP(stage string) (string, error) {
	var b strings.Builder
	stageProto := filepath.Join(stage, "proto")
	err := filepath.WalkDir(stageProto, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".proto") {
			return err
		}

		rel, err := filepath.Rel(stage, path)
		if err != nil {
			return fmt.Errorf("filepath.Rel: %w", err)
		}

		fmt.Fprintf(&b, ",M%s=github.com/OffchainLabs/prysm/v7/%s", rel, filepath.Dir(rel))

		return nil
	})
	if err != nil {
		return "", err
	}

	b.WriteString(",Mgoogle/api/annotations.proto=google.golang.org/genproto/googleapis/api/annotations")
	b.WriteString(",Mgoogle/api/http.proto=google.golang.org/genproto/googleapis/api/annotations")

	return b.String(), nil
}

func pkgProtos(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("readDir %s: %w", dir, err)
	}

	var out []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".proto") {
			continue
		}

		p := filepath.ToSlash(filepath.Join(dir, e.Name()))
		out = append(out, p)
	}

	sort.Strings(out)

	return out, nil
}
