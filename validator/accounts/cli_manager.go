package accounts

import (
	"context"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/api/client"
	"github.com/prysmaticlabs/prysm/v5/api/client/beacon/node"
	"github.com/prysmaticlabs/prysm/v5/api/client/beacon/validator_api"
	grpcutil "github.com/prysmaticlabs/prysm/v5/api/grpc"
	"github.com/prysmaticlabs/prysm/v5/crypto/bls"
	"github.com/prysmaticlabs/prysm/v5/validator/accounts/wallet"
	validatorHelpers "github.com/prysmaticlabs/prysm/v5/validator/helpers"
	"github.com/prysmaticlabs/prysm/v5/validator/keymanager"
	"github.com/prysmaticlabs/prysm/v5/validator/keymanager/derived"
	"google.golang.org/grpc"
)

// NewCLIManager allows for managing validator accounts via CLI commands.
func NewCLIManager(opts ...Option) (*CLIManager, error) {
	acc := &CLIManager{
		mnemonicLanguage: derived.DefaultMnemonicLanguage,
		inputReader:      os.Stdin,
	}
	for _, opt := range opts {
		if err := opt(acc); err != nil {
			return nil, err
		}
	}
	return acc, nil
}

// CLIManager defines a struct capable of performing various validator
// wallet & account operations via the command line.
type CLIManager struct {
	wallet               *wallet.Wallet
	keymanager           keymanager.IKeymanager
	keymanagerKind       keymanager.Kind
	showPrivateKeys      bool
	listValidatorIndices bool
	deletePublicKeys     bool
	importPrivateKeys    bool
	readPasswordFile     bool
	skipMnemonicConfirm  bool
	dialOpts             []grpc.DialOption
	grpcHeaders          []string
	beaconRPCProvider    string
	walletKeyCount       int
	privateKeyFile       string
	passwordFilePath     string
	keysDir              string
	mnemonicLanguage     string
	backupsDir           string
	backupsPassword      string
	filteredPubKeys      []bls.PublicKey
	rawPubKeys           [][]byte
	formattedPubKeys     []string
	exitJSONOutputPath   string
	walletDir            string
	walletPassword       string
	mnemonic             string
	numAccounts          int
	mnemonic25thWord     string
	beaconApiEndpoint    string
	beaconApiTimeout     time.Duration
	inputReader          io.Reader
}

func (acm *CLIManager) prepareBeaconClients(ctx context.Context) (validator_api.Client, node.Client, error) {
	if acm.dialOpts == nil {
		return nil, nil, errors.New("failed to construct dial options for beacon clients")
	}

	ctx = grpcutil.AppendHeaders(ctx, acm.grpcHeaders)
	grpcConn, err := grpc.DialContext(ctx, acm.beaconRPCProvider, acm.dialOpts...)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "could not dial endpoint %s", acm.beaconRPCProvider)
	}
	conn := validatorHelpers.NewNodeConnection(
		grpcConn,
		acm.beaconApiEndpoint,
		acm.beaconApiTimeout,
	)

	restHandler := client.NewBeaconApiJsonRestHandler(
		http.Client{Timeout: acm.beaconApiTimeout},
		acm.beaconApiEndpoint,
	)
	validatorClient := validator_api.NewClient(conn, restHandler)
	nodeClient := node.NewClient(conn, restHandler)

	return validatorClient, nodeClient, nil
}
