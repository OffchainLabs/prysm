package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"runtime"
	"strings"

	"github.com/OffchainLabs/prysm/v7/io/file"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

var log = logrus.WithField("prefix", "node")

// ConfirmAction uses the passed in actionText as the confirmation text displayed in the terminal.
// The user must enter Y or N to indicate whether they confirm the action detailed in the warning text.
// Returns a boolean representing the user's answer.
func ConfirmAction(actionText, deniedText string) (bool, error) {
	var confirmed bool
	reader := bufio.NewReader(os.Stdin)
	log.Warn(actionText)

	for {
		fmt.Print(">> ")

		line, _, err := reader.ReadLine()
		if err != nil {
			return false, err
		}
		trimmedLine := strings.TrimSpace(string(line))
		lineInput := strings.ToUpper(trimmedLine)
		if lineInput != "Y" && lineInput != "N" {
			log.Errorf("Invalid option of %s chosen, please only enter Y/N", line)
			continue
		}
		if lineInput == "Y" {
			confirmed = true
			break
		}
		log.Warn(deniedText)
		break
	}

	return confirmed, nil
}

// EnterPassword queries the user for their password through the terminal, in order to make sure it is
// not passed in a visible way to the terminal.
func EnterPassword(confirmPassword bool, pr PasswordReader) (string, error) {
	var passphrase string
	log.Info("Enter a password:")
	bytePassword, err := pr.ReadPassword()
	if err != nil {
		return "", errors.Wrap(err, "could not read account password")
	}
	text := bytePassword
	passphrase = strings.ReplaceAll(text, "\n", "")
	if confirmPassword {
		log.Info("Please re-enter your password:")
		bytePassword, err := pr.ReadPassword()
		if err != nil {
			return "", errors.Wrap(err, "could not read account password")
		}
		text := bytePassword
		confirmedPass := strings.ReplaceAll(text, "\n", "")
		if passphrase != confirmedPass {
			log.Info("Passwords did not match, please try again")
			return EnterPassword(true, pr)
		}
	}
	return passphrase, nil
}

// ExpandSingleEndpointIfFile expands the path for --execution-provider if specified as a file.
func ExpandSingleEndpointIfFile(ctx *cli.Context, flag *cli.StringFlag) error {
	// Return early if no flag value is set.
	if !ctx.IsSet(flag.Name) {
		return nil
	}
	// Return early for non-unix operating systems, as there is
	// no shell path expansion for ipc endpoints on windows.
	if runtime.GOOS == "windows" {
		return nil
	}
	web3endpoint := ctx.String(flag.Name)
	switch {
	case strings.HasPrefix(web3endpoint, "http://"):
	case strings.HasPrefix(web3endpoint, "https://"):
	case strings.HasPrefix(web3endpoint, "ws://"):
	case strings.HasPrefix(web3endpoint, "wss://"):
	default:
		web3endpoint, err := file.ExpandPath(ctx.String(flag.Name))
		if err != nil {
			return errors.Wrapf(err, "could not expand path for %s", ctx.String(flag.Name))
		}
		if err := ctx.Set(flag.Name, web3endpoint); err != nil {
			return errors.Wrapf(err, "could not set %s to %s", flag.Name, web3endpoint)
		}
	}
	return nil
}

// ParseVModule parses a comma-separated list of package=level entries.
func ParseVModule(input string) (map[string]logrus.Level, logrus.Level, error) {
	var l logrus.Level
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return nil, l, nil
	}

	parts := strings.Split(trimmed, ",")
	result := make(map[string]logrus.Level, len(parts))
	for _, raw := range parts {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			return nil, l, fmt.Errorf("invalid vmodule entry: empty segment")
		}
		kv := strings.Split(entry, "=")
		if len(kv) != 2 {
			return nil, l, fmt.Errorf("invalid vmodule entry %q: expected path=level", entry)
		}
		pkg := strings.TrimSpace(kv[0])
		levelText := strings.TrimSpace(kv[1])
		if pkg == "" {
			return nil, l, fmt.Errorf("invalid vmodule entry %q: empty package path", entry)
		}
		if levelText == "" {
			return nil, l, fmt.Errorf("invalid vmodule entry %q: empty level", entry)
		}
		if strings.Contains(pkg, "*") {
			return nil, l, fmt.Errorf("invalid vmodule package path %q: wildcards are not allowed", pkg)
		}
		if strings.ContainsAny(pkg, " \t\n") {
			return nil, l, fmt.Errorf("invalid vmodule package path %q: whitespace is not allowed", pkg)
		}
		if strings.HasPrefix(pkg, "/") {
			return nil, l, fmt.Errorf("invalid vmodule package path %q: leading slash is not allowed", pkg)
		}
		cleaned := path.Clean(pkg)
		if cleaned != pkg || pkg == "." || pkg == ".." {
			return nil, l, fmt.Errorf("invalid vmodule package path %q: must be an absolute package path. (trailing slash not allowed)", pkg)
		}
		if _, exists := result[pkg]; exists {
			return nil, l, fmt.Errorf("invalid vmodule package path %q: duplicate entry", pkg)
		}
		level, err := logrus.ParseLevel(levelText)
		if err != nil {
			return nil, l, fmt.Errorf("invalid vmodule level %q: must be one of panic, fatal, error, warn, info, debug, trace", levelText)
		}
		result[pkg] = level
	}

	maxLevel := logrus.PanicLevel
	for _, lvl := range result {
		if lvl > maxLevel {
			maxLevel = lvl
		}
	}

	return result, maxLevel, nil
}
