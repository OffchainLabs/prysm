package beacon_api

import (
	"bytes"
	"context"
	"fmt"
	"net/http"

	"github.com/OffchainLabs/prysm/v7/network/httputil"
	"github.com/OffchainLabs/prysm/v7/validator/client/beacon-api/mock"
	"github.com/pkg/errors"
	"go.uber.org/mock/gomock"
)

// expectPostSSZWithFallback adapts the existing request-body assertions to the Handler method.
func expectPostSSZWithFallback(handler *mock.MockHandler) {
	handler.EXPECT().PostSSZWithFallback(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
	).DoAndReturn(func(
		ctx context.Context,
		endpoint string,
		headers map[string]string,
		sszFn func() ([]byte, error),
		jsonFn func() ([]byte, error),
	) error {
		postJSON := func() error {
			body, err := jsonFn()
			if err != nil {
				return fmt.Errorf("marshal JSON body: %w", err)
			}

			if err := handler.Post(ctx, endpoint, headers, bytes.NewBuffer(body), nil); err != nil {
				return fmt.Errorf("post JSON: %w", err)
			}
			return nil
		}

		body, err := sszFn()
		if err != nil {
			return fmt.Errorf("marshal SSZ body: %w", err)
		}

		if err = handler.PostSSZ(ctx, endpoint, headers, bytes.NewBuffer(body)); err == nil {
			return nil
		}
		if !errors.Is(err, &httputil.DefaultJsonError{Code: http.StatusUnsupportedMediaType}) {
			return fmt.Errorf("post SSZ: %w", err)
		}

		return postJSON()
	}).AnyTimes()
}
