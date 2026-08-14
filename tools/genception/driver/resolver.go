package driver

import (
	"os"
	"strings"
)

// pathResolverFunc rewrites a single bazel-symbolic source path into a real,
// absolute filesystem path. newPathResolver returns an implementation.
type pathResolverFunc func(path string) string

func newPathResolver(env environment) pathResolverFunc {
	res := &pathResolver{
		execRoot:   env.pwd,
		outputBase: env.packagesBase,
	}
	return res.resolve
}

type pathResolver struct {
	outputBase string
	execRoot   string
}

const (
	prefixExecRoot   = "__BAZEL_EXECROOT__"
	prefixOutputBase = "__BAZEL_OUTPUT_BASE__"
	prefixWorkspace  = "__BAZEL_WORKSPACE__"
)

var prefixes = []string{prefixExecRoot, prefixOutputBase, prefixWorkspace}

func (r pathResolver) resolve(path string) string {
	for _, prefix := range prefixes {
		if strings.HasPrefix(path, prefix) {
			for _, rpl := range []string{r.execRoot, r.outputBase} {
				rp := strings.Replace(path, prefix, rpl, 1)
				_, err := os.Stat(rp)
				if err == nil {
					return rp
				}
			}
			return path
		}
	}
	log.WithField("path", path).Warn("unrecognized path prefix when resolving source paths in json import metadata")
	return path
}
