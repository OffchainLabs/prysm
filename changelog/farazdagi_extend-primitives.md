### Changed

- Extended `Slot` and `Epoch` primitives with new methods (absolute diff, capped/floored variants)
- Common code is abstracted in a single module (dedup w/o any performance hit)
- Code is benchmarked, with typed methods (`AddSlot`, `MulSlot`, `DivEpoch` etc) having 40% improved performance
