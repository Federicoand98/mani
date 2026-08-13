package app

import (
	"context"
	"errors"
	"testing"
)

func TestUnavailableClient_ReturnsOriginalError(t *testing.T) {
	want := errors.New("no creds")
	c := unavailableClient{err: want}
	_, err := c.Send(context.Background(), nil, nil, nil)
	if !errors.Is(err, want) {
		t.Fatalf("got %v, want %v", err, want)
	}
}
