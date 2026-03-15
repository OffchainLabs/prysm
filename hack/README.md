# Bash Scripts

This subproject contains useful bash scripts for working with our repository. We have a simple tool that outputs coverage, update Go protobuf generated files, and more.


## update-go-pbs.sh

This script generates the *.pb.go files from the *.proto files using `protoc`.
After running `update-go-pbs.sh` keep only the *.pb.go for the protos that have changed before checking in.
*Note*: the generated files may not have imports correctly linted and will need to be fixed to remove associated errors.
