// Package event defines the Publisher interface for sending banner events to an external
// message broker. The interface is used by the service layer (CORE) to remain dependency-free.
package event

import (
	"context"

	"github.com/VadimVikt/banner-rotation/internal/model"
)

// Publisher sends domain events to an external message broker.
type Publisher interface {
	Publish(ctx context.Context, ev model.Event) error
}

// NOPublisher discards all events. Useful for tests.
type NOPublisher struct{}

// Publish implements Publisher but does nothing.
func (NOPublisher) Publish(_ context.Context, _ model.Event) error {
	return nil
}

// Ensure NOPublisher implements Publisher at compile time.
var _ Publisher = NOPublisher{}