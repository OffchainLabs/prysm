package cmd

import (
	"flag"
	"os"
	"os/user"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/urfave/cli/v2"
)

func TestExpandSingleEndpointIfFile(t *testing.T) {
	app := cli.App{}
	set := flag.NewFlagSet("test", 0)
	ExecutionEndpointFlag := &cli.StringFlag{Name: "execution-endpoint", Value: ""}
	set.String(ExecutionEndpointFlag.Name, "", "")
	context := cli.NewContext(&app, set, nil)

	// with nothing set
	require.NoError(t, ExpandSingleEndpointIfFile(context, ExecutionEndpointFlag))
	require.Equal(t, "", context.String(ExecutionEndpointFlag.Name))

	// with url scheme
	require.NoError(t, context.Set(ExecutionEndpointFlag.Name, "http://localhost:8545"))
	require.NoError(t, ExpandSingleEndpointIfFile(context, ExecutionEndpointFlag))
	require.Equal(t, "http://localhost:8545", context.String(ExecutionEndpointFlag.Name))

	// relative user home path
	usr, err := user.Current()
	require.NoError(t, err)
	require.NoError(t, context.Set(ExecutionEndpointFlag.Name, "~/relative/path.ipc"))
	require.NoError(t, ExpandSingleEndpointIfFile(context, ExecutionEndpointFlag))
	require.Equal(t, usr.HomeDir+"/relative/path.ipc", context.String(ExecutionEndpointFlag.Name))

	// current dir path
	curentdir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, context.Set(ExecutionEndpointFlag.Name, "./path.ipc"))
	require.NoError(t, ExpandSingleEndpointIfFile(context, ExecutionEndpointFlag))
	require.Equal(t, curentdir+"/path.ipc", context.String(ExecutionEndpointFlag.Name))
}
