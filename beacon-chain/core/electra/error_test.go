package electra

import (
	"testing"

	"github.com/pkg/errors"
)

func TestIsExecutionRequestError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "random error",
			err:  errors.New("some error"),
			want: false,
		},
		{
			name: "execution request error",
			err:  execReqErr{errors.New("execution request failed")},
			want: true,
		},
		{
			name: "wrapped execution request error",
			err:  errors.Wrap(execReqErr{errors.New("execution request failed")}, "wrapped"),
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsExecutionRequestError(tt.err)
			if got != tt.want {
				t.Errorf("IsExecutionRequestError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
