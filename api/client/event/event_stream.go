package event

import (
	"bufio"
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/api"
	"github.com/prysmaticlabs/prysm/v5/api/client"
	log "github.com/sirupsen/logrus"
)

const (
	EventHead                        = "head"
	EventBlock                       = "block"
	EventAttestation                 = "attestation"
	EventVoluntaryExit               = "voluntary_exit"
	EventBlsToExecutionChange        = "bls_to_execution_change"
	EventProposerSlashing            = "proposer_slashing"
	EventAttesterSlashing            = "attester_slashing"
	EventFinalizedCheckpoint         = "finalized_checkpoint"
	EventChainReorg                  = "chain_reorg"
	EventContributionAndProof        = "contribution_and_proof"
	EventLightClientFinalityUpdate   = "light_client_finality_update"
	EventLightClientOptimisticUpdate = "light_client_optimistic_update"
	EventPayloadAttributes           = "payload_attributes"
	EventBlobSidecar                 = "blob_sidecar"
	EventError                       = "error"
	EventConnectionError             = "connection_error"
)

var (
	_ = EventStreamClient(&EventStream{})
)

var DefaultEventTopics = []string{EventHead}

type EventStreamClient interface {
	Subscribe(eventsChannel chan<- *Event)
}

type Event struct {
	EventType string
	Data      []byte
}

// EventStream is responsible for subscribing to the Beacon API events endpoint
// and dispatching received events to subscribers.
type EventStream struct {
	ctx        context.Context
	httpClient *http.Client
	host       string
	topics     []string
}

func NewEventStream(ctx context.Context, httpClient *http.Client, host string, topics []string) (*EventStream, error) {
	// Check if the host is a valid URL
	_, err := url.ParseRequestURI(host)
	if err != nil {
		return nil, err
	}
	if len(topics) == 0 {
		return nil, errors.New("no topics provided")
	}

	return &EventStream{
		ctx:        ctx,
		httpClient: httpClient,
		host:       host,
		topics:     topics,
	}, nil
}

func (h *EventStream) Subscribe(eventsChannel chan<- *Event) {
	allTopics := strings.Join(h.topics, ",")
	log.WithField("topics", allTopics).Info("Listening to Beacon API events")
	fullUrl := h.host + "/eth/v1/events?topics=" + allTopics
	req, err := http.NewRequestWithContext(h.ctx, http.MethodGet, fullUrl, nil)
	if err != nil {
		eventsChannel <- &Event{
			EventType: EventConnectionError,
			Data:      []byte(errors.Wrap(err, "failed to create HTTP request").Error()),
		}
	}
	req.Header.Set("Accept", api.EventStreamMediaType)
	req.Header.Set("Connection", api.KeepAlive)
	resp, err := h.httpClient.Do(req)
	if err != nil {
		eventsChannel <- &Event{
			EventType: EventConnectionError,
			Data:      []byte(errors.Wrap(err, client.ErrConnectionIssue.Error()).Error()),
		}
		return
	}

	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			log.WithError(closeErr).Error("Failed to close events response body")
		}
	}()
	// Create a new scanner to read lines from the response body
	scanner := bufio.NewScanner(resp.Body)
	// Set the split function for the scanning operation
	scanner.Split(scanLinesWithCarriage)

	var eventType, data string // Variables to store event type and data

	// Iterate over lines of the event stream
	for scanner.Scan() {
		select {
		case <-h.ctx.Done():
			log.Info("Context canceled, stopping event stream")
			close(eventsChannel)
			return
		default:
			line := scanner.Text()
			// Handle the event based on your specific format
			if line == "" {
				// Empty line indicates the end of an event
				if eventType != "" && data != "" {
					// Process the event when both eventType and data are set
					eventsChannel <- &Event{EventType: eventType, Data: []byte(data)}
				}

				// Reset eventType and data for the next event
				eventType, data = "", ""
				continue
			}
			et, ok := strings.CutPrefix(line, "event: ")
			if ok {
				// Extract event type from the "event" field
				eventType = et
			}
			d, ok := strings.CutPrefix(line, "data: ")
			if ok {
				// Extract data from the "data" field
				data = d
			}
		}
	}

	if err := scanner.Err(); err != nil {
		eventsChannel <- &Event{
			EventType: EventConnectionError,
			Data:      []byte(errors.Wrap(err, errors.Wrap(client.ErrConnectionIssue, "scanner failed").Error()).Error()),
		}
	}
}
