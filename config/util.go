package config

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	"github.com/ethereum/go-ethereum/common"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/yaml"
)

func UnmarshalFromURL(ctx context.Context, from string, to interface{}) error {
	u, err := url.ParseRequestURI(from)
	if err != nil {
		return err
	}
	if u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("invalid URL: %s", from)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, from, nil)
	if err != nil {
		return errors.Wrap(err, "failed to create http request")
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return errors.Wrap(err, "failed to send http request")
	}
	defer func(Body io.ReadCloser) {
		err = Body.Close()
		if err != nil {
			log.WithError(err).Error("Failed to close response body")
		}
	}(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return errors.Errorf("http request to %v failed with status code %d", from, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&to); err != nil {
		return errors.Wrap(err, "failed to decode http response")
	}
	return nil
}

func UnmarshalFromFile(from string, to interface{}) error {
	cleanpath := filepath.Clean(from)
	b, err := os.ReadFile(cleanpath)
	if err != nil {
		return errors.Wrap(err, "failed to open file")
	}

	if err := yaml.Unmarshal(b, to); err != nil {
		return errors.Wrap(err, "failed to unmarshal yaml file")
	}
	return nil
}

func WarnNonChecksummedAddress(feeRecipient string) error {
	mixedcaseAddress, err := common.NewMixedcaseAddressFromString(feeRecipient)
	if err != nil {
		return errors.Wrapf(err, "could not decode fee recipient %s", feeRecipient)
	}
	if !mixedcaseAddress.ValidChecksum() {
		log.Warnf("Fee recipient %s is not a checksum Ethereum address. "+
			"The checksummed address is %s and will be used as the fee recipient. "+
			"We recommend using a mixed-case address (checksum) "+
			"to prevent spelling mistakes in your fee recipient Ethereum address", feeRecipient, mixedcaseAddress.Address().Hex())
	}
	return nil
}
