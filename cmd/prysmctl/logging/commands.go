package logging

import (
	"errors"

	"github.com/urfave/cli/v2"
)

var Commands = []*cli.Command{
	{
		Name:    "logs",
		Aliases: []string{"l", "logging"},
		Usage:   "Translate logs from fluentd or json to unstructured text logs",
		Action: func(ctx *cli.Context) error {
			// TODO: Add flags `--from=fluentd` and `--to=text` where the default is from fluentd to text and the options are fluentd, text, json for from/to.
			// TODO: Ingest messages from stdin, send them to TranslateFluentdtoUnstructuredLog, then print the result to stdout.
			return errors.New("not implemented")
		},
	},
}
