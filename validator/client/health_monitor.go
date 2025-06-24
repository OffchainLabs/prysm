package client

import (
	"context"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/validator/client/iface"
	"github.com/sirupsen/logrus"
)

type healthMonitor struct {
	ctx       context.Context
	cancel    context.CancelFunc
	v         iface.Validator
	maxFails  int
	healthyCh chan bool // emits true → healthy, false → unhealthy
	fails     int
	isHealthy bool
	sync.RWMutex
}

// newHealthMonitor
func newHealthMonitor(
	parentCtx context.Context,
	parentCancel context.CancelFunc,
	maxFails int,
	v iface.Validator,
) *healthMonitor {
	m := &healthMonitor{
		ctx:       parentCtx,
		cancel:    parentCancel,
		maxFails:  maxFails,
		v:         v,
		healthyCh: make(chan bool, 1),
	}

	// Prime channel with the current status so consumers have an initial value.
	isHealthy := v.FindHealthyHost(parentCtx)
	m.isHealthy = isHealthy
	m.healthyCh <- isHealthy
	if !isHealthy {
		m.fails = 1
	}

	return m
}

func (m *healthMonitor) IsHealthy() bool {
	m.RLock()
	defer m.RUnlock()
	return m.isHealthy
}

func (m *healthMonitor) performHealthCheck() {
	m.Lock()
	defer m.Unlock()

	ishealthy := m.v.FindHealthyHost(m.ctx)
	if ishealthy {
		m.fails = 0
	} else if m.maxFails > 0 && m.fails < m.maxFails {
		log.WithFields(logrus.Fields{
			"fails":    m.fails,
			"maxFails": m.maxFails,
		}).Debug("Failed health check, beacon node is unresponsive")
		m.fails++
	} else if m.maxFails > 0 && m.fails >= m.maxFails {
		log.WithField("max_fails", m.maxFails).Debug("Maximum health checksreached. Stopping health check routine")
		m.isHealthy = ishealthy
		m.cancel()
		return
	}
	if ishealthy == m.isHealthy {
		// is not a new status so skip update
		return
	}
	m.isHealthy = ishealthy
	// Non-blocking send to channel
	select {
	case m.healthyCh <- ishealthy:
	default:
		// Channel full, drain and send
		<-m.healthyCh
		m.healthyCh <- ishealthy
	}
}

func (m *healthMonitor) loop() {
	log.Info("Starting health check routine for beacon node apis")
	// just check one a slot
	interval := time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second
	ticker := time.NewTicker(interval)

	// Initial check immediately
	m.performHealthCheck()

	// Continue periodic checks
	for {
		select {
		case <-ticker.C:
			m.performHealthCheck()
		case <-m.ctx.Done():
			log.Debug("Context canceled, stopping health checking")
			return
		}
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
