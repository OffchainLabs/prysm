# Kurtosis scripts for EIP-8025

## How to run

I slightly modified [Manu's tip](https://hackmd.io/8z4thpsyQJioaU6jj0Wazw) by adding those in my `~/.zshrc`.

```zsh
# Kurtosis Aliases
blog() {
    docker logs -f "$(docker ps | grep cl-"$1"-prysm-geth | awk '{print $NF}')" 2>&1
}

vlog() {
    docker logs -f "$(docker ps | grep vc-"$1"-geth-prysm | awk '{print $NF}')" 2>&1
} 

dora() {
    open http://localhost:$(docker ps --format '{{.Ports}} {{.Names}}' | awk '/dora/ {split($1, a, "->"); split(a[1], b, ":"); print b[2]}')
}

graf() {
    open http://localhost:$(docker ps --format '{{.Ports}} {{.Names}}' | awk '/grafana/ {split($1, a, "->"); split(a[1], b, ":"); print b[2]}')
}

devnet () {
    local args_file_path="./kurtosis/default.yaml"
    if [ ! -z "$1" ]; then
        args_file_path="$1"
        echo "Using custom args-file path: $args_file_path"
    else
        echo "Using default args-file path: $args_file_path"
    fi

    kurtosis clean -a && 
    bazel build //cmd/beacon-chain:oci_image_tarball --platforms=@io_bazel_rules_go//go/toolchain:linux_arm64_cgo --config=release &&
    docker load -i bazel-bin/cmd/beacon-chain/oci_image_tarball/tarball.tar && 
    docker tag gcr.io/offchainlabs/prysm/beacon-chain prysm-bn-custom-image &&
    bazel build //cmd/validator:oci_image_tarball --platforms=@io_bazel_rules_go//go/toolchain:linux_arm64_cgo --config=release &&
    docker load -i bazel-bin/cmd/validator/oci_image_tarball/tarball.tar && 
    docker tag gcr.io/offchainlabs/prysm/validator prysm-vc-custom-image &&
    kurtosis run github.com/ethpandaops/ethereum-package --args-file="$args_file_path" --verbosity brief && 
    dora
}

stop() {
    kurtosis clean -a
}

dps() {
    docker ps --format "table {{.ID}}\\t{{.Image}}\\t{{.Status}}\\t{{.Names}}" -a
}
```

At the project directory, you can simply spin up a devnet with:

```bash
$ devnet
```

Or you can specify the network parameter YAML file like:

```bash
$ devnet ./kurtosis/proof_verify.yaml
```

### Running scripts with local images

Images from Prysm can be automatically loaded from `devnet` command, but if you want to run a script with `lighthouse`:

#### `./kurtosis/interop.yaml`

- `lighthouse:local`: Please build your own image following [Lighthouse's guide](https://lighthouse-book.sigmaprime.io/installation_docker.html?highlight=docker#building-the-docker-image) on [`kevaundray/kw/sel-alternative`](https://github.com/kevaundray/lighthouse/tree/kw/sel-alternative/) branch.
