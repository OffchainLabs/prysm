# Prysm — Ethereum Consensus Layer Client

Bazel is the first-class build system — always build and test with Bazel.

## Skills

The "how" lives in `.claude/skills/` — invoke these instead of running commands by hand:

- `/precheck` — format, gazelle, code generators, build. Run before every commit.
- `/test` — unit tests for affected packages. For any tests you add or modify, run this command.

## Architecture

Main executables in `cmd/` including `beacon-chain`, `validator`.

- `beacon-chain/` — core chain: state transition (`core/`, per fork), block processing & fork choice (`blockchain/`), `state/`, `db/`, `p2p/`, `sync/`, RPC/API
- `validator/` — key management, slashing protection, duties
- `consensus-types/` — shared consensus type definitions
- `proto/` — protobuf defs + generated code
- `api/` — REST + gRPC
- `config/` — chain params (`params/`) and feature flags (`features/`)
- `testing/` — test utilities, spec conformance, end-to-end

Prysm implements the [Ethereum consensus specs](https://github.com/ethereum/consensus-specs/tree/master/specs) (organized by fork - phase0, altair, bellatrix, capella, deneb, electra, fulu, gloas, etc) — they are the source of truth for protocol behavior. Each containing:

- *beacon-chain.md* — state transition / core logic
- *fork-choice.md* — fork choice rules
- *p2p-interface.md* — networking / gossip

## Conventions

- **Always run `/precheck` before every commit — it must pass** (format, gazelle, generators, build).
- Branch from and target `develop`.
- Every PR needs a changelog fragment: `changelog/<github_user>_<branch_name>.md` (managed by `unclog`).
- Keep comments short — one line, no multi-line explanations.
- Verify tests you add or modify with `/test`.
