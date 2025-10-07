### Fixed
- Don't include entries in the fork schedule if their epoch is set to far future epoch. Avoids reporting next_fork_version == <unscheduled fork>.
