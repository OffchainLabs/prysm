### Fixed

- Stop downscoring and disconnecting honest peers when a data column sidecar response references a block the node does not hold locally (a forked/behind node would otherwise self-isolate); recovery fetches now request columns by root.
