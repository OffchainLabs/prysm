package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestParseNolint(t *testing.T) {
	tests := []struct {
		comment   string
		ok        bool
		analyzers map[string]bool
	}{
		{comment: "// comment"},
		{comment: "//nolint", ok: true},
		{comment: "//nolint:all", ok: true},
		{comment: "// nolint:foo", ok: true, analyzers: map[string]bool{"foo": true}},
		{comment: "//nolint:foo // the reason", ok: true, analyzers: map[string]bool{"foo": true}},
		{comment: "// nolint:a,b,c", ok: true, analyzers: map[string]bool{"a": true, "b": true, "c": true}},
		{comment: "//nolint:a,all", ok: true},
	}

	for _, tt := range tests {
		t.Run(tt.comment, func(t *testing.T) {
			analyzers, ok := parseNolint(tt.comment)
			require.Equal(t, tt.ok, ok)
			require.DeepEqual(t, tt.analyzers, analyzers)
		})
	}
}

func TestNolintSilenced(t *testing.T) {
	const src = `package p

// f is silenced for gocognit, on its whole body.
// nolint:gocognit
func f() {
	_ = 1
	_ = 2
}

func g() {
	_ = 3 //nolint
	_ = 4
}
`

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "p.go", src, parser.ParseComments)
	require.NoError(t, err)

	tf := fset.File(f.Pos())
	pos := func(line int) token.Pos { return tf.LineStart(line) + 1 }

	var c nolintCache
	tests := []struct {
		line     int
		analyzer string
		silenced bool
	}{
		{line: 6, analyzer: "gocognit", silenced: true},
		{line: 7, analyzer: "gocognit", silenced: true},
		{line: 7, analyzer: "errcheck", silenced: false},
		{line: 11, analyzer: "errcheck", silenced: true},
		{line: 12, analyzer: "errcheck", silenced: false},
	}

	for _, tt := range tests {
		require.Equal(t, tt.silenced, c.silenced(fset, []*ast.File{f}, tt.analyzer, pos(tt.line)), "line %d %s", tt.line, tt.analyzer)
	}
}
