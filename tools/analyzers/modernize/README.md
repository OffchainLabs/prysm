# Modernize

These are analyzers that are re-exported from golang.org/x/tools/go/analysis/passes/modernize.

nogo expects a package to contain a single analyzer, while modernize exposes the named analyzers and an analyzer suite. This is incompatible with nogo so we have re-exported each analyzer.
