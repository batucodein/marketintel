package internalsched

import (
	"context"

	"github.com/batuhan/marketintel/internal/outreach/poller"
)

// PollerComponent wraps the existing inbound poller so it can hook into the
// scheduler tick. PollOnce already iterates every channel; we just count the
// number of channels touched as the "processed" metric.
type PollerComponent struct {
	P *poller.Poller
}

func (c *PollerComponent) Tick(ctx context.Context) (TickResult, error) {
	c.P.PollOnce(ctx)
	return TickResult{Component: "inbound_poller"}, nil
}
