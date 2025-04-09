load("@prysm//tools/go:def.bzl", "go_repository")  # gazelle:keep

def distroless_deps():
    go_repository(
        name = "com_github_ulikunitz_xz",
        importpath = "github.com/ulikunitz/xz",
        sum = "h1:kpFauv27b6ynzBNT/Xy+1k+fK4WswhN/6PN5WhFAGw8=",
        version = "v0.5.11",
    )
    
    go_repository(
        name = "com_github_spdx_tools_golang",
        importpath = "github.com/spdx/tools-golang",
        sum = "h1:9B623Cfs+mclYK6dsae7gLSwuIBHvlgmEup87qpqsAQ=",
        version = "v0.3.1-0.20230104082527-d6f58551be3f",
    )

