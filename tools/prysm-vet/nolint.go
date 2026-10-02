package main

import (
	"go/ast"
	"go/token"
	"strings"
	"sync"
)

// nolintRange is a line range where a `//nolint[:a,b]` comment silences diagnostics.
type nolintRange struct {
	filename string
	from, to int

	// analyzers are the silenced analyzers; nil means all of them.
	analyzers map[string]bool
}

// nolintCache holds the nolint ranges of each file, shared by all the analyzers of a package.
type nolintCache struct {
	mu     sync.Mutex
	ranges map[*ast.File][]nolintRange
}

// silenced reports whether a nolint comment in files silences analyzer at pos.
func (c *nolintCache) silenced(fset *token.FileSet, files []*ast.File, analyzer string, pos token.Pos) bool {
	p := fset.Position(pos)
	for _, f := range files {
		if pos < f.FileStart || pos > f.FileEnd {
			continue
		}

		for _, r := range c.of(fset, f) {
			if r.filename != p.Filename || p.Line < r.from || p.Line > r.to {
				continue
			}

			if r.analyzers == nil || r.analyzers[analyzer] {
				return true
			}
		}
	}

	return false
}

// of returns the nolint ranges of f, computed once.
func (c *nolintCache) of(fset *token.FileSet, f *ast.File) []nolintRange {
	c.mu.Lock()
	defer c.mu.Unlock()

	if ranges, ok := c.ranges[f]; ok {
		return ranges
	}

	if c.ranges == nil {
		c.ranges = make(map[*ast.File][]nolintRange)
	}

	ranges := nolintRanges(fset, f)
	c.ranges[f] = ranges

	return ranges
}

// nolintRanges returns the nolint ranges of f, as rules_go's nogo computes them: a comment
// silences the whole node it is attached to (see ast.NewCommentMap), e.g. a function when
// placed on its doc comment, or a multi-line statement when trailing it.
func nolintRanges(fset *token.FileSet, f *ast.File) []nolintRange {
	var ranges []nolintRange
	for node, groups := range ast.NewCommentMap(fset, f, f.Comments) {
		from, to := fset.Position(node.Pos()), fset.Position(node.End())
		for _, group := range groups {
			for _, comment := range group.List {
				analyzers, ok := parseNolint(comment.Text)
				if !ok {
					continue
				}

				ranges = append(ranges, nolintRange{filename: from.Filename, from: from.Line, to: to.Line, analyzers: analyzers})
			}
		}
	}

	return ranges
}

// parseNolint parses a nolint comment and returns the silenced analyzers, or nil when it
// silences all of them. Copied from rules_go (go/tools/builders/nolint.go, Apache 2.0).
func parseNolint(text string) (map[string]bool, bool) {
	text = strings.TrimLeft(text, "/ ")
	if !strings.HasPrefix(text, "nolint") {
		return nil, false
	}

	// Strip explanation comments.
	text = strings.TrimSpace(strings.Split(text, "//")[0])

	parts := strings.Split(text, ":")
	if len(parts) == 1 {
		return nil, true
	}

	result := map[string]bool{}
	for linter := range strings.SplitSeq(parts[1], ",") {
		if strings.EqualFold(linter, "all") {
			return nil, true
		}

		result[linter] = true
	}

	return result, true
}
