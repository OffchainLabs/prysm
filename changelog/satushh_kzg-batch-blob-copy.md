### Changed

- Avoid a redundant 128 KiB blob copy and heap allocation in the KZG batch verification path by copying each blob directly into the preallocated destination.
