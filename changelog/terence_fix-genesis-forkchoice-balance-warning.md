### Fixed

- Demoted genesis node pending balance underflow warning to debug in forkchoice. The ePBS pending/full balance split causes a harmless accounting mismatch at genesis where there is no separate payload delivery.
