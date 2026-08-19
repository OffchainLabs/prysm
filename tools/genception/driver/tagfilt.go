package driver

import (
	"go/build"
	"path/filepath"
)

// tagFilterFunc keeps only the files that the active build tags would include.
// goTagFilter returns an implementation.
type tagFilterFunc func([]string) []string

// goTagFilter returns a function that filters files based on the build tags
// specified in the environment. It uses the go/build package to determine
// whether a file would be included in the build/compilation context.
func goTagFilter(env environment) tagFilterFunc {
	bctx := build.Default
	bctx.BuildTags = env.goTags
	return func(files []string) []string {
		ret := make([]string, 0, len(files))
		for _, f := range files {
			dir, filename := filepath.Split(f)
			ext := filepath.Ext(f)
			if ext == "" {
				ret = append(ret, f)
				continue
			}
			match, err := bctx.MatchFile(dir, filename)
			if err != nil {
				log.WithError(err).WithField("file", f).Warn("error matching file")
			}
			if match {
				ret = append(ret, f)
			}
		}
		return ret
	}
}
