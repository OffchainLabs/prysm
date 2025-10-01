package logging

import (
	"bufio"
	"fmt"
	"io"
	"os"

	"github.com/urfave/cli/v2"
)

var Commands = []*cli.Command{
	{
		Name:    "logs",
		Aliases: []string{"l", "logging"},
		Usage:   "Translate logs from fluentd or json to unstructured text logs",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "from",
				Usage: "Input log format (fluentd, text, json)",
				Value: "fluentd",
			},
			&cli.StringFlag{
				Name:  "to",
				Usage: "Output log format (fluentd, text, json)",
				Value: "text",
			},
		},
		Action: func(ctx *cli.Context) error {
			from := ctx.String("from")
			to := ctx.String("to")

			// Validate flags
			validFormats := map[string]bool{"fluentd": true, "text": true, "json": true}
			if !validFormats[from] {
				return fmt.Errorf("invalid --from format: %s. Must be one of: fluentd, text, json", from)
			}
			if !validFormats[to] {
				return fmt.Errorf("invalid --to format: %s. Must be one of: fluentd, text, json", to)
			}

			// Only fluentd to text is currently implemented
			if from != "fluentd" || to != "text" {
				return fmt.Errorf("only fluentd to text translation is currently supported")
			}

			// Read from stdin line by line
			scanner := bufio.NewScanner(os.Stdin)
			for scanner.Scan() {
				line := scanner.Text()
				if line == "" {
					continue
				}

				// Translate the log line
				translated, err := TranslateFluentdtoUnstructuredLog(line)
				if err != nil {
					// Write error to stderr and continue processing
					fmt.Fprintf(os.Stderr, "Error translating line: %v\n", err)
					continue
				}

				// Write to stdout (without extra newline as TranslateFluentdtoUnstructuredLog adds one)
				if _, err := io.WriteString(os.Stdout, translated); err != nil {
					return fmt.Errorf("failed to write output: %w", err)
				}
			}

			if err := scanner.Err(); err != nil {
				return fmt.Errorf("error reading input: %w", err)
			}

			return nil
		},
	},
}
