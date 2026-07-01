### Fixed

- Increment the `builder_exits_processed_total` metric when a builder exit is initiated. The counter was defined but never incremented, so it always reported zero.
