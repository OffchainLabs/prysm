# Web3Signer

Web3Signer is a popular remote signer tool by Consensys to allow users to store validation keys outside the validation
client and signed without the vc knowing the private keys. Web3Signer Specs are found by
searching `Consensys' Web3Signer API specification`

issue: https://github.com/prysmaticlabs/prysm/issues/9994

API interface: https://github.com/ethereum/remote-signing-api

## Features

### CLI

detailed info found on https://docs.prylabs.network/docs/wallet/web3signer

Flags used on validator client
- `--validators-external-signer-url=http://localhost:9000`

with hex keys
- `--validators-external-signer-public-keys=0xa99a...e44c,0xb89b...4a0b`

with url
- `--validators-external-signer-public-keys=https://web3signer.com/api/v1/eth2/publicKeys`

### Key sources

Public keys are tracked per source and the validating set is their union. Every key has
exactly one owner, and a source only ever replaces its own set:

- `--validators-external-signer-public-keys` (flag): fixed at startup.
- `--validators-external-signer-url` public keys endpoint: replaced whole on every poll.
- `--validators-external-signer-key-file`: written by the operator and by the keymanager API.

Because a source only replaces its own set, `--validators-external-signer-poll-interval` works
whether or not a key file is configured: a poll swaps the URL's keys and leaves the flag and
file keys alone.

The keymanager API therefore only mutates the key file source. `POST /eth/v1/remotekeys`
needs a key file, otherwise there is no permanent storage to import into and it reports
`error`. `DELETE /eth/v1/remotekeys` reports `error` for keys owned by the flag or the URL,
because they would come straight back on the next reload.

### API

- Get Public keys: returns all public keys currently stored with web3signer excluding newly added keys if reload keys
  was not run.
- Sign: Signs a message with a given public key. There are several types of messages that can be signed ( web3signer
  type to prysm type):
    - BLOCK <- *validatorpb.SignRequest_Block
    - ATTESTATION <- *validatorpb.SignRequest_AttestationData
    - AGGREGATE_AND_PROOF <- *validatorpb.SignRequest_AggregateAttestationAndProof
    - AGGREGATION_SLOT <- *validatorpb.SignRequest_Slot
    - BLOCK_ALTAIR <- *validatorpb.SignRequest_BlockAltair
    - BLOCK_BELLATRIX <- *validatorpb.SignRequest_BlockBellatrix
    - BLINDED_BLOCK_BELLATRIX <- *validatorpb.SignRequest_BlindedBlockBellatrix
    - DEPOSIT <- not supported
    - RANDAO_REVEAL <- *validatorpb.SignRequest_Epoch
    - VOLUNTARY_EXIT <- *validatorpb.SignRequest_Exit
    - SYNC_COMMITTEE_MESSAGE <- *validatorpb.SignRequest_SyncMessageBlockRoot
    - SYNC_COMMITTEE_SELECTION_PROOF <- *validatorpb.SignRequest_SyncAggregatorSelectionData
    - SYNC_COMMITTEE_CONTRIBUTION_AND_PROOF <- *validatorpb.SignRequest_ContributionAndProof
- Reload Keys: reloads all public keys from the web3signer.
- Get Server Status: returns OK if the web3signer is ok.

## Files Added and Files Changed

- Files Added:
    - validator/keymanager/remote-web3signer package

- Files Modified:
    - modified:   cmd/validator/flags/flags.go
    - modified:   validator/accounts/accounts_backup.go
    - modified:   validator/accounts/accounts_list.go
    - modified:   validator/accounts/iface/wallet.go
    - modified:   validator/accounts/userprompt/prompt.go
    - modified:   validator/accounts/wallet/wallet.go
    - modified:   validator/accounts/wallet_create.go
    - modified:   validator/client/runner.go
    - modified:   validator/client/validator.go
    - modified:   validator/keymanager/remote-web3signer/keymanager.go
    - modified:   validator/keymanager/types.go