### Fixed

- Reject duplicate data column sidecars in `DataColumnSidecarsByRange` responses by enforcing a strict `(slot, column_index)` order, and in `DataColumnSidecarsByRoot` responses by rejecting an already received `(block_root, index)`.
