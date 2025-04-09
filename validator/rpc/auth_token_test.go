package rpc

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/golang-jwt/jwt/v4"
	"github.com/prysmaticlabs/prysm/v5/api"
	"github.com/prysmaticlabs/prysm/v5/io/file"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func setupWalletDir(t testing.TB) string {
	walletDir := filepath.Join(t.TempDir(), "wallet")
	require.NoError(t, os.MkdirAll(walletDir, os.ModePerm))
	return walletDir
}

func TestServer_AuthenticateUsingExistingToken(t *testing.T) {
	// Initializing for the first time, there is no auth token file in
	// the wallet directory, so we generate a jwt token and secret from scratch.
	walletDir := setupWalletDir(t)
	authTokenPath := filepath.Join(walletDir, api.AuthTokenFileName)
	srv := &Server{
		authTokenPath: authTokenPath,
	}

	err := srv.initializeAuthToken()
	require.NoError(t, err)

	unaryInfo := &grpc.UnaryServerInfo{
		FullMethod: "Proto.CreateWallet",
	}
	unaryHandler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return nil, nil
	}
	ctxMD := map[string][]string{
		"authorization": {"Bearer " + srv.authToken},
	}
	ctx := context.Background()
	ctx = metadata.NewIncomingContext(ctx, ctxMD)
	_, err = srv.AuthTokenInterceptor()(ctx, "xyz", unaryInfo, unaryHandler)
	require.NoError(t, err)

	// Next up, we make the same request but reinitialize the server and we should still
	// pass with the same auth token.
	srv = &Server{
		authTokenPath: authTokenPath,
	}
	err = srv.initializeAuthToken()
	require.NoError(t, err)
	_, err = srv.AuthTokenInterceptor()(ctx, "xyz", unaryInfo, unaryHandler)
	require.NoError(t, err)
}

func TestServer_RefreshAuthTokenOnFileChange(t *testing.T) {
	// Initializing for the first time, there is no auth token file in
	// the wallet directory, so we generate a jwt token and secret from scratch.
	walletDir := setupWalletDir(t)
	authTokenPath := filepath.Join(walletDir, api.AuthTokenFileName)
	srv := &Server{
		authTokenPath: authTokenPath,
	}

	err := srv.initializeAuthToken()
	require.NoError(t, err)
	currentToken := srv.authToken

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.refreshAuthTokenFromFileChanges(ctx, srv.authTokenPath)

	// Wait for service to be ready.
	time.Sleep(time.Millisecond * 250)

	// Update the auth token file with a new secret.
	require.NoError(t, CreateAuthToken(srv.authTokenPath, "localhost:7500"))

	// The service should have picked up the file change and set the jwt secret to the new one.
	time.Sleep(time.Millisecond * 500)
	newToken := srv.authToken
	require.Equal(t, true, currentToken != newToken)
	err = os.Remove(srv.authTokenPath)
	require.NoError(t, err)
}

// TODO: remove this test when legacy files are removed
func TestServer_LegacyTokensStillWork(t *testing.T) {
	hook := logTest.NewGlobal()
	// Initializing for the first time, there is no auth token file in
	// the wallet directory, so we generate a jwt token and secret from scratch.
	walletDir := setupWalletDir(t)
	authTokenPath := filepath.Join(walletDir, api.AuthTokenFileName)

	bytesBuf := new(bytes.Buffer)
	_, err := bytesBuf.WriteString("b5bbbaf533b625a93741978857f13d7adeca58445a1fb00ecf3373420b92776c")
	require.NoError(t, err)
	_, err = bytesBuf.WriteString("\n")
	require.NoError(t, err)
	_, err = bytesBuf.WriteString("eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.e30.MxwOozSH-TLbW_XKepjyYDHm2IT8Ki0tD3AHuajfNMg")
	require.NoError(t, err)
	_, err = bytesBuf.WriteString("\n")
	require.NoError(t, err)
	err = file.MkdirAll(walletDir)
	require.NoError(t, err)

	err = file.WriteFile(authTokenPath, bytesBuf.Bytes())
	require.NoError(t, err)

	srv := &Server{
		authTokenPath: authTokenPath,
	}

	err = srv.initializeAuthToken()
	require.NoError(t, err)

	require.Equal(t, hexutil.Encode(srv.jwtSecret), "0xb5bbbaf533b625a93741978857f13d7adeca58445a1fb00ecf3373420b92776c")
	require.Equal(t, srv.authToken, "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.e30.MxwOozSH-TLbW_XKepjyYDHm2IT8Ki0tD3AHuajfNMg")

	f, err := os.Open(filepath.Clean(srv.authTokenPath))
	require.NoError(t, err)

	scanner := bufio.NewScanner(f)
	var lines []string

	// Scan the file and collect lines, excluding empty lines
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	require.Equal(t, len(lines), 2)
	require.LogsContain(t, hook, "Auth token does not follow our standards and should be regenerated")
	// Check for scanning errors
	err = scanner.Err()
	require.NoError(t, err)
	err = f.Close()
	require.NoError(t, err)

	err = os.Remove(srv.authTokenPath)
	require.NoError(t, err)
}

// TODO: remove this test when legacy files are removed
func TestServer_LegacyTokensBadSecret(t *testing.T) {
	// Initializing for the first time, there is no auth token file in
	// the wallet directory, so we generate a jwt token and secret from scratch.
	walletDir := setupWalletDir(t)
	authTokenPath := filepath.Join(walletDir, api.AuthTokenFileName)

	bytesBuf := new(bytes.Buffer)
	_, err := bytesBuf.WriteString("----------------")
	require.NoError(t, err)
	_, err = bytesBuf.WriteString("\n")
	require.NoError(t, err)
	_, err = bytesBuf.WriteString("eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.e30.MxwOozSH-TLbW_XKepjyYDHm2IT8Ki0tD3AHuajfNMg")
	require.NoError(t, err)
	_, err = bytesBuf.WriteString("\n")
	require.NoError(t, err)
	err = file.MkdirAll(walletDir)
	require.NoError(t, err)

	err = file.WriteFile(authTokenPath, bytesBuf.Bytes())
	require.NoError(t, err)

	srv := &Server{
		authTokenPath: authTokenPath,
	}

	err = srv.initializeAuthToken()
	require.ErrorContains(t, "could not decode JWT secret", err)
}

func Test_initializeAuthToken(t *testing.T) {
	// Initializing for the first time, there is no auth token file in
	// the wallet directory, so we generate a jwt token and secret from scratch.
	walletDir := setupWalletDir(t)
	authTokenPath := filepath.Join(walletDir, api.AuthTokenFileName)
	srv := &Server{
		authTokenPath: authTokenPath,
	}
	err := srv.initializeAuthToken()
	require.NoError(t, err)

	// Initializing second time, we generate something from the initial file.
	srv2 := &Server{
		authTokenPath: authTokenPath,
	}
	err = srv2.initializeAuthToken()
	require.NoError(t, err)
	require.Equal(t, srv.authToken, srv2.authToken)

	// Deleting the auth token and re-initializing means we create a jwt token
	// and secret from scratch again.
	walletDir = setupWalletDir(t)
	authTokenPath = filepath.Join(walletDir, api.AuthTokenFileName)
	srv3 := &Server{
		authTokenPath: authTokenPath,
	}

	err = srv3.initializeAuthToken()
	require.NoError(t, err)
	require.NotEqual(t, srv.authToken, srv3.authToken)
}

// "createTokenString" now uses jwt.RegisteredClaims instead of jwt.StandardClaims (deprecated),
// make sure empty jwt.RegisteredClaims and empty jwt.StandardClaims generates the same token.
func Test_UseRegisteredClaimInsteadOfStandClaims(t *testing.T) {
	jwtsecret, err := hex.DecodeString("12345678900123456789abcdeffedcba")
	require.NoError(t, err)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.StandardClaims{}) // jwt.StandardClaims is deprecated
	wantedTokenString, err := token.SignedString(jwtsecret)
	require.NoError(t, err)

	gotTokenString, err := createTokenString(jwtsecret)
	require.NoError(t, err)

	if wantedTokenString != gotTokenString {
		t.Errorf("%s != %s", wantedTokenString, gotTokenString)
	}
}
