package dispatcher

import (
	"context"
	"fmt"
	"io"
)

type Handler func(context.Context, []string) error

type Config struct {
	BeaconChain Handler
	Geth        Handler
	Stdout      io.Writer
	Stderr      io.Writer
}

func Run(ctx context.Context, args []string, cfg Config) error {
	subcommand, rest := SplitArgs(args)
	switch subcommand {
	case "", "help", "--help", "-h":
		PrintUsage(writerOrDiscard(cfg.Stdout))
		return nil
	case "beacon-chain":
		if cfg.BeaconChain == nil {
			return fmt.Errorf("beacon-chain runner is not configured")
		}
		return cfg.BeaconChain(ctx, append([]string{"beacon-chain"}, rest...))
	case "geth":
		if cfg.Geth == nil {
			return fmt.Errorf("geth runner is not configured")
		}
		return cfg.Geth(ctx, append([]string{"geth"}, rest...))
	default:
		return fmt.Errorf("unknown prysm-node subcommand %q\n\nRun 'prysm-node --help' for usage", subcommand)
	}
}

func SplitArgs(args []string) (string, []string) {
	if len(args) < 2 {
		return "", nil
	}
	return args[1], args[2:]
}

func PrintUsage(w io.Writer) {
	fmt.Fprint(w, `Usage: prysm-node <command> [options]

Commands:
  beacon-chain  Run the Prysm beacon node
  geth          Run a standalone Geth execution node
  help          Show this help
`)
}

func writerOrDiscard(w io.Writer) io.Writer {
	if w == nil {
		return io.Discard
	}
	return w
}
