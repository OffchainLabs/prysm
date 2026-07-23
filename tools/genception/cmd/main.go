package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"github.com/OffchainLabs/prysm/v7/tools/genception/driver"
)

var log = driver.Logger

func run(_ context.Context, in io.Reader, out io.Writer, args []string) error {
	// NewJSONDriver builds the (IO-heavy) package registry once per invocation.
	pd, err := driver.NewJSONDriver()
	if err != nil {
		return fmt.Errorf("unable to load JSON files: %w", err)
	}
	// Logged after construction so it lands in the configured log file.
	log.WithField("args", strings.Join(args, " ")).Info("genception lookup")
	// Note: we are returning all files required to build a specific package.
	// For file queries (`file=`), this means that the CompiledGoFiles will
	// include more than the only file being specified.
	resp, err := pd.Handle(in, args)
	if err != nil {
		log.WithError(err).Error("unable to handle driver request")
	}
	_, err = out.Write(resp)
	if err != nil {
		log.WithError(err).Error("unable to write driver response")
	}
	return err
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := run(ctx, os.Stdin, os.Stdout, os.Args[1:]); err != nil {
		_, err := fmt.Fprintf(os.Stderr, "error: %v", err)
		if err != nil {
			log.WithError(err).Error("unhandled error in package resolution")
		}
		// gopls will check the packages driver exit code, and if there is an
		// error, it will fall back to go list. Obviously we don't want that,
		// so force a 0 exit code.
		os.Exit(0)
	}
}
