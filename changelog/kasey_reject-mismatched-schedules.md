### Changed
- Reject incoming connections when the fork schedule of the connecting peer (parsed from their ENR) has a matching next_fork_epoch, but mismatched next_fork_version or nfd (next fork digest).
