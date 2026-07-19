package event

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestStreamGuard_Replace(t *testing.T) {
	var g StreamGuard

	// First run: gets a live context.
	ctx1, finish1 := g.Replace(t.Context())
	require.NoError(t, ctx1.Err())
	g.MarkRunning(true)
	require.Equal(t, true, g.IsRunning())

	// Simulate the first run exiting when its context is canceled.
	exited := make(chan struct{})
	go func() {
		<-ctx1.Done()
		g.MarkRunning(false)
		finish1()
		close(exited)
	}()

	// Replacing cancels the first run and waits for it to finish.
	ctx2, finish2 := g.Replace(t.Context())
	<-exited
	require.NotNil(t, ctx1.Err())
	require.NoError(t, ctx2.Err())
	finish2()
}
