package client

import (
	"context"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/async/event"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/sirupsen/logrus"
)

// healthClient is the slice of the beacon node client that the monitor probes.
type healthClient interface {
	Host() string
	EnsureReady(ctx context.Context) bool
}

type healthMonitor struct {
	ctx             context.Context
	cancel          context.CancelFunc
	client          healthClient
	maxFails        int
	healthyCh       chan bool // emits true → healthy, false → unhealthy
	healthEventFeed *event.Feed
	probeFeed       event.Feed // every verdict, where healthEventFeed carries only changes
	fails           int
	isHealthy       bool
	sync.RWMutex
}

// newHealthMonitor
func newHealthMonitor(
	parentCtx context.Context,
	parentCancel context.CancelFunc,
	maxFails int,
	client healthClient,
) *healthMonitor {
	m := &healthMonitor{
		ctx:             parentCtx,
		cancel:          parentCancel,
		maxFails:        maxFails,
		client:          client,
		healthyCh:       make(chan bool),
		healthEventFeed: new(event.Feed),
	}
	m.healthEventFeed.Subscribe(m.healthyCh)
	return m
}

func (m *healthMonitor) IsHealthy() bool {
	m.RLock()
	defer m.RUnlock()
	return m.isHealthy
}

func (m *healthMonitor) performHealthCheck() {
	ishealthy := m.client.EnsureReady(m.ctx)
	m.updateStatus(ishealthy)
	m.probeFeed.Send(ishealthy)
}

func (m *healthMonitor) updateStatus(ishealthy bool) {
	m.Lock()
	defer m.Unlock()
	if ishealthy {
		m.fails = 0
	} else if m.maxFails > 0 && m.fails < m.maxFails {
		log.WithFields(logrus.Fields{
			"fails":    m.fails,
			"maxFails": m.maxFails,
			"url":      api.RedactEndpointList(m.client.Host()),
		}).Warn("Failed health check, beacon node is unresponsive")
		m.fails++
	} else if m.maxFails > 0 && m.fails >= m.maxFails {
		log.WithFields(logrus.Fields{
			"maxFails": m.maxFails,
			"url":      api.RedactEndpointList(m.client.Host()),
		}).Warn("Maximum health checks reached. Stopping health check routine")
		m.isHealthy = ishealthy
		m.cancel()
		return
	}
	if ishealthy == m.isHealthy {
		// is not a new status so skip update
		log.WithFields(logrus.Fields{
			"isHealthy": m.isHealthy,
			"url":       api.RedactEndpointList(m.client.Host()),
		}).Debug("Health status did not change")
		return
	}
	log.WithFields(logrus.Fields{
		"current":  ishealthy,
		"previous": m.isHealthy,
		"url":      api.RedactEndpointList(m.client.Host()),
	}).Info("Health status changed")
	m.isHealthy = ishealthy
	go m.healthEventFeed.Send(ishealthy) // non blocking send
}

func (m *healthMonitor) loop() {
	log.Debug("Starting health check routine for beacon node apis")
	ticker := time.NewTicker(params.BeaconConfig().SlotDuration())

	for ; true; <-ticker.C { // check immediately
		if m.ctx.Err() != nil {
			log.Debug("Context canceled, stopping health checking")
			return
		}
		m.performHealthCheck()
	}
}

// Start launches the monitor loop (non-blocking).
func (m *healthMonitor) Start() {
	go m.loop()
}

// Stop terminates the monitor and closes its channel.
func (m *healthMonitor) Stop() {
	m.cancel()
}

// HealthyChan exposes liveness updates; the channel closes when Stop() is called.
func (m *healthMonitor) HealthyChan() <-chan bool { return m.healthyCh }

// WaitForHealthy blocks until a probe completed after the call reports the beacon node healthy.
func (m *healthMonitor) WaitForHealthy(ctx context.Context) error {
	verdicts := make(chan bool, 1)
	sub := m.probeFeed.Subscribe(verdicts)
	defer sub.Unsubscribe()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case healthy := <-verdicts:
			if healthy {
				return nil
			}
			log.WithField("url", api.RedactEndpointList(m.client.Host())).Warn("Beacon node still unhealthy, waiting before retrying")
		}
	}
}
