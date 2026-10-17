### Fixed

- Return from `PostSSZWithFallback` as soon as one beacon node accepts the write instead of waiting for every node, so a slow or unreachable node no longer delays validator client submissions.
