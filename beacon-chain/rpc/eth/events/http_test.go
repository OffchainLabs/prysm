package events

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

type StreamingResponseWriterRecorder struct {
	http.ResponseWriter
	r             io.Reader
	w             io.Writer
	statusWritten *int
	status        chan int
	bodyRecording []byte
	flushed       bool
	writeDeadline time.Time
	ctx           context.Context
}

func (w *StreamingResponseWriterRecorder) StatusChan() chan int {
	return w.status
}

func NewStreamingResponseWriterRecorder(ctx context.Context) *StreamingResponseWriterRecorder {
	r, w := io.Pipe()
	return &StreamingResponseWriterRecorder{
		ResponseWriter: httptest.NewRecorder(),
		r:              r,
		w:              w,
		status:         make(chan int, 1),
		ctx:            ctx,
	}
}

// Write implements http.ResponseWriter.
func (w *StreamingResponseWriterRecorder) Write(data []byte) (int, error) {
	w.WriteHeader(http.StatusOK)
	written, err := writeWithDeadline(w.ctx, w.w, data, w.writeDeadline)
	if err != nil {
		return written, err
	}
	// The test response writer is non-blocking.
	return w.ResponseWriter.Write(data)
}

var zeroTimeValue = time.Time{}

func writeWithDeadline(ctx context.Context, w io.Writer, data []byte, deadline time.Time) (int, error) {
	result := struct {
		written int
		err     error
	}{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		result.written, result.err = w.Write(data)
	}()
	if deadline == zeroTimeValue {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-done:
			return result.written, result.err
		}
	}
	select {
	case <-time.After(time.Until(deadline)):
		return 0, http.ErrHandlerTimeout
	case <-done:
		return result.written, result.err
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

// WriteHeader implements http.ResponseWriter.
func (w *StreamingResponseWriterRecorder) WriteHeader(statusCode int) {
	if w.statusWritten != nil {
		return
	}
	w.statusWritten = &statusCode
	w.status <- statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *StreamingResponseWriterRecorder) Body() io.Reader {
	return w.r
}

func (w *StreamingResponseWriterRecorder) RequireStatus(t *testing.T, status int) {
	if w.statusWritten == nil {
		t.Fatal("WriteHeader was not called")
	}
	require.Equal(t, status, *w.statusWritten)
}

func (w *StreamingResponseWriterRecorder) Flush() {
	w.WriteHeader(200)
	fw, ok := w.ResponseWriter.(http.Flusher)
	if ok {
		fw.Flush()
	}
	w.flushed = true
}

func (w *StreamingResponseWriterRecorder) SetWriteDeadline(d time.Time) error {
	w.writeDeadline = d
	return nil
}

var _ http.ResponseWriter = &StreamingResponseWriterRecorder{}
