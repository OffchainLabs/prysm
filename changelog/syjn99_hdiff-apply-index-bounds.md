### Fixed

- hdiff: `ApplyDiff` now rejects a diff whose prefix-drop index (pending deposits, pending partial withdrawals, pending consolidations, builder pending withdrawals) or builder index exceeds the source length, instead of panicking with a slice-bounds error or appending nil builders.
