### Changed

- Moved the broadcast and event notifier logic for saving LC updates to the store function.
- Fixed the issue with broadcasting more than twice per LC Finality update, and the if-case bug.
- Separated the finality update validation rules for saving and broadcasting.