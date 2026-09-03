### Fixed

- Fix `GetValidatorQueue` sorting the activation and exit queues by queue position instead of validator index, which returned misordered `ActivationPublicKeys`, `ActivationValidatorIndices`, `ExitPublicKeys` and `ExitValidatorIndices`.
