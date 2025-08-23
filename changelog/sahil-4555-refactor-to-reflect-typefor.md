### Added

Replaced reflect.TypeOf with reflect.TypeFor to make the code cleaner and safer. Unlike TypeOf, which needs a runtime value (often a dummy or zero value), TypeFor works directly with the type at compile time. This avoids extra allocations, improves type safety, and is especially useful in generic code.
