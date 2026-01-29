# Specification References

This directory contains specification reference tracking files managed by
[ethspecify](https://github.com/jtraglia/ethspecify).

## Installation

Install `ethspecify` with the following command:

```bash
pipx install ethspecify
```

> [!NOTE]
> You can run `ethspecify <cmd>` in the `specrefs` directory or
> `ethspecify <cmd> --path=specrefs` from the project's root directory.

## Maintenance

When adding support for a new specification version, follow these steps:

0. Change directory into the `specrefs` directory.
1. Update the version in `.ethspecify.yml` configuration.
2. Run `ethspecify process` to update/populate specrefs.
3. Run `ethspecify check` to check specrefs.
4. If there are errors, use the error message as a guide to fix the issue. If
   there are new specrefs with empty sources, implement/locate each item and
   update each specref source list. If you choose not to implement an item,
   add an exception to the appropriate section the the `.ethspecify.yml`
   configuration.
5. Repeat steps 3 and 4 until `ethspecify check` passes.
6. Run `git diff` to view updated specrefs. If an object/function/etc has
   changed, make the necessary updates to the implementation.
7. Lastly, in the project's root directory, run `act -j check-specrefs` to
   ensure everything is correct.
