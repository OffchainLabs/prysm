package rpc

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/OffchainLabs/prysm/v6/api"
	"github.com/OffchainLabs/prysm/v6/config/features"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v6/io/file"
	"github.com/fsnotify/fsnotify"
	"github.com/golang-jwt/jwt/v4"
	"github.com/pkg/errors"
)

// CreateAuthToken generates a new jwt key, token and writes them
// to a file in the specified directory. Also, it logs out a prepared URL
// for the user to navigate to and authenticate with the Prysm web interface.
func CreateAuthToken(authPath, validatorWebAddr string) error {
	token, err := api.GenerateRandomHexString()
	if err != nil {
		return err
	}
	log.Infof("Generating auth token and saving it to %s", authPath)
	if err := saveAuthToken(authPath, token); err != nil {
		return err
	}
	logValidatorWebAuth(validatorWebAddr, token, authPath)
	return nil
}

// Upon launch of the validator client, we initialize an auth token by either creating
// one from scratch or reading it from a file. This token can then be shown to the
// user via stdout and the validator client should then attempt to open the default
// browser. The web interface authenticates by looking for this token in the query parameters
// of the URL. This token is then used as the bearer token for jwt auth.
func (s *Server) initializeAuthToken() error {
	if s.authTokenPath == "" {
		return errors.New("auth token path is empty")
	}
	exists, err := file.Exists(s.authTokenPath, file.Regular)
	if err != nil {
		return errors.Wrapf(err, "could not check if file %s exists", s.authTokenPath)
	}
	if exists {
		f, err := os.Open(filepath.Clean(s.authTokenPath))
		if err != nil {
			return err
		}
		defer func() {
			if err := f.Close(); err != nil {
				log.Error(err)
			}
		}()
		secret, token, err := readAuthTokenFile(f)
		if err != nil {
			return err
		}
		s.jwtSecret = secret
		s.authToken = token
		return nil
	}
	token, err := api.GenerateRandomHexString()
	if err != nil {
		return err
	}
	s.authToken = token
	return saveAuthToken(s.authTokenPath, token)
}

func (s *Server) refreshAuthTokenFromFileChanges(ctx context.Context, authTokenPath string) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.WithError(err).Error("Could not initialize file watcher")
		return
	}
	defer func() {
		if err := watcher.Close(); err != nil {
			log.WithError(err).Error("Could not close file watcher")
		}
	}()
	if err := watcher.Add(authTokenPath); err != nil {
		log.WithError(err).Errorf("Could not add file %s to file watcher", authTokenPath)
		return
	}
	for {
		select {
		case event := <-watcher.Events:
			if event.Op.String() == "REMOVE" {
				log.Error("Auth Token was removed! Restart the validator client to regenerate a token")
				s.authToken = ""
				continue
			}
			// If a file was modified, we attempt to read that file
			// and parse it into our accounts store.
			if err := s.initializeAuthToken(); err != nil {
				log.WithError(err).Errorf("Could not watch for file changes for: %s", authTokenPath)
				continue
			}
			validatorWebAddr := fmt.Sprintf("%s:%d", s.httpHost, s.httpPort)
			logValidatorWebAuth(validatorWebAddr, s.authToken, authTokenPath)
		case err := <-watcher.Errors:
			log.WithError(err).Errorf("Could not watch for file changes for: %s", authTokenPath)
		case <-ctx.Done():
			return
		}
	}
}

func logValidatorWebAuth(validatorWebAddr, token, tokenPath string) {
	if features.Get().EnableWeb {
		webAuthURLTemplate := "http://%s/initialize?token=%s"
		webAuthURL := fmt.Sprintf(
			webAuthURLTemplate,
			validatorWebAddr,
			url.QueryEscape(token),
		)
		log.Infof(
			"Starting Prysm WebUI, once your validator process is running, navigate to the link below to authenticate",
		)
		log.Info(webAuthURL)
	}
	log.Infof("Validator Client auth token for gRPC and REST authentication set at %s", tokenPath)
}

func saveAuthToken(tokenPath string, token string) error {
	bytesBuf := new(bytes.Buffer)
	if _, err := bytesBuf.WriteString(token); err != nil {
		return err
	}
	if _, err := bytesBuf.WriteString("\n"); err != nil {
		return err
	}

	if err := file.MkdirAll(filepath.Dir(tokenPath)); err != nil {
		return errors.Wrapf(err, "could not create directory %s", filepath.Dir(tokenPath))
	}
	if err := file.WriteFile(tokenPath, bytesBuf.Bytes()); err != nil {
		return errors.Wrapf(err, "could not write to file %s", tokenPath)
	}

	return nil
}

func readAuthTokenFile(r io.Reader) ([]byte, string, error) {
	scanner := bufio.NewScanner(r)
	var lines []string
	var secret []byte
	var token string
	// Scan the file and collect lines, excluding empty lines
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}

	// Check for scanning errors
	if err := scanner.Err(); err != nil {
		return nil, "", err
	}

	// Process based on the number of lines, excluding empty ones
	switch len(lines) {
	case 1:
		// If there is only one line, interpret it as the token
		token = strings.TrimSpace(lines[0])
	case 2:
		// TODO: Deprecate after a few releases
		// For legacy files
		// If there are two lines, the first is the jwt key and the second is the token
		jwtKeyHex := strings.TrimSpace(lines[0])
		s, err := hex.DecodeString(jwtKeyHex)
		if err != nil {
			return nil, "", errors.Wrapf(err, "could not decode JWT secret")
		}
		secret = bytesutil.SafeCopyBytes(s)
		token = strings.TrimSpace(lines[1])
		log.Warn("Auth token is a legacy file and should be regenerated.")
	default:
		return nil, "", errors.New("Auth token file format has multiple lines, please update the auth token to a single line that is a 256 bit hex string")
	}
	if err := api.ValidateAuthToken(token); err != nil {
		log.WithError(err).Warn("Auth token does not follow our standards and should be regenerated either \n" +
			"1. by removing the current token file and restarting \n" +
			"2. using the `validator web generate-auth-token` command. \n" +
			"Tokens can be generated through the `validator web generate-auth-token` command")
	}
	return secret, token, nil
}

// Creates a JWT token string using the JWT key.
func createTokenString(jwtKey []byte) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{})
	// Sign and get the complete encoded token as a string using the secret
	tokenString, err := token.SignedString(jwtKey)
	if err != nil {
		return "", err
	}
	return tokenString, nil
}
