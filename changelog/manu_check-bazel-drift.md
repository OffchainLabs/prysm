### Ignored

- Add `./hack/check-bazel-drift.sh` to regenerate proto/SSZ files via Bazel and fail on any drift from the committed `make gen` output beyond the two known-benign differences (protoc version header and the leading `//go:build !minimal` constraint). Run it in the `check-generated-go` CI workflow.
