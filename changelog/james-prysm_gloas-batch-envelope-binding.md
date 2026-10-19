### Fixed

- Initial sync now marks a Gloas block's payload as present only after it verifies that block's payload envelope. A batch with an envelope that matches none of its blocks is rejected.
