# Contributing to Prysm

Questions? Reach us on [Discord](https://discord.gg/qEZK94mFXP).

## Workflow

> [!IMPORTANT]
> **Every pull request must fix an issue assigned to its author.** Pull requests from external contributors that don't meet this rule are closed automatically.

1. **Open an issue** describing the bug or feature, or find an [existing one](https://github.com/OffchainLabs/prysm/issues).
2. **Wait to be assigned.** Comment on the issue to say you want to work on it. Once the approach is agreed upon, a maintainer assigns it to you.
3. **Open a pull request** against `develop` that references the issue it fixes in its description.

## Development

### Install Bazel

Install [Bazelisk](https://github.com/bazelbuild/bazelisk), which downloads the Bazel version pinned in [`.bazelversion`](.bazelversion):

```sh
# macOS
brew install bazelisk

# Linux (amd64)
curl -L https://github.com/bazelbuild/bazelisk/releases/latest/download/bazelisk-linux-amd64 -o /usr/local/bin/bazel
chmod +x /usr/local/bin/bazel

# Linux (arm64)
curl -L https://github.com/bazelbuild/bazelisk/releases/latest/download/bazelisk-linux-arm64 -o /usr/local/bin/bazel
chmod +x /usr/local/bin/bazel
```

Verify the installation with `bazel version`.

### Run the beacon node and the validator client

List the beacon node flags:

```sh
bazel run //cmd/beacon-chain -- --help
```

List the validator client flags:

```sh
bazel run //cmd/validator -- --help
```

### Run tests

```sh
# All tests in a package
bazel test //beacon-chain/blockchain:go_default_test

# A specific test
bazel test //beacon-chain/blockchain:go_default_test --test_filter='^TestStore_OnBlockBatch$'

# With the minimal preset
bazel test //beacon-chain/blockchain:go_default_test --config=minimal

# All tests
bazel test //...
```

Add `--test_output=errors` to print the logs of failing tests.

## Changelog

Each pull request must add a changelog fragment file in the [`changelog`](changelog) directory, named `<github username>_<branch name>.md`. Its format is described in the [unclog README](https://github.com/OffchainLabs/unclog?tab=readme-ov-file#what-is-a-changelog-fragment).


## Pull request checklist

Before opening a pull request, make sure that:

- [ ] The PR fixes an issue assigned to me, and the issue is referenced in the description.
- [ ] The description explains what the PR does and why, with enough context for reviewers.
- [ ] New features and bug fixes come with tests, written like the existing ones.
- [ ] I tested my changes and described how in the description (if applicable).
- [ ] The PR includes a [changelog fragment](#changelog).
- [ ] I signed the Contributor License Agreement (CLA). 


