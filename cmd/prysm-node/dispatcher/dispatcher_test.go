package dispatcher

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestSplitArgs(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		subcommand string
		rest       []string
	}{
		{
			name: "empty",
		},
		{
			name: "program only",
			args: []string{"prysm-node"},
		},
		{
			name:       "subcommand",
			args:       []string{"prysm-node", "beacon-chain", "--help"},
			subcommand: "beacon-chain",
			rest:       []string{"--help"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			subcommand, rest := SplitArgs(tt.args)
			if subcommand != tt.subcommand {
				t.Fatalf("subcommand = %q, want %q", subcommand, tt.subcommand)
			}
			if strings.Join(rest, "\x00") != strings.Join(tt.rest, "\x00") {
				t.Fatalf("rest = %v, want %v", rest, tt.rest)
			}
		})
	}
}

func TestRunHelp(t *testing.T) {
	var out bytes.Buffer
	if err := Run(context.Background(), []string{"prysm-node", "--help"}, Config{Stdout: &out}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Usage: prysm-node") {
		t.Fatalf("help output = %q, want usage", out.String())
	}
}

func TestRunBeaconChain(t *testing.T) {
	var gotArgs []string
	err := Run(context.Background(), []string{"prysm-node", "beacon-chain", "--help"}, Config{
		BeaconChain: func(_ context.Context, args []string) error {
			gotArgs = append([]string(nil), args...)
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"beacon-chain", "--help"}
	if strings.Join(gotArgs, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("args = %v, want %v", gotArgs, want)
	}
}

func TestRunUnknownSubcommand(t *testing.T) {
	err := Run(context.Background(), []string{"prysm-node", "wat"}, Config{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), `unknown prysm-node subcommand "wat"`) {
		t.Fatalf("error = %q, want unknown subcommand", err)
	}
}
