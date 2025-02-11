### Fixed

- We change how we track blob indexes during their reconstruction from the EL to prevent
a mutating blob mask from causing invalid sidecars from being created.