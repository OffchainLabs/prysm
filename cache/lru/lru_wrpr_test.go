package lru

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNew(t *testing.T) {
	assert.NotPanics(t, func() {
		New(10)
	})
}

func TestNew_ZeroOrNegativeSize(t *testing.T) {
	assert.Panics(t, func() {
		New(0)
	})
	assert.Panics(t, func() {
		New(-1)
	})
}

func TestNewWithEvict(t *testing.T) {
	assert.NotPanics(t, func() {
		NewWithEvict(10, func(key any, value any) {})
	})
}

func TestNewWithEvict_ZeroOrNegativeSize(t *testing.T) {
	assert.Panics(t, func() {
		NewWithEvict(0, func(key any, value any) {})
	})
	assert.Panics(t, func() {
		NewWithEvict(-1, func(key any, value any) {})
	})
}
