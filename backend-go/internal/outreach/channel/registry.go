package channel

import (
	"context"
	"fmt"
	"sync"

	"github.com/batuhan/marketintel/internal/domain"
)

// Registry maps channel-type strings to factories. Package-level singleton.
type Registry struct {
	mu        sync.RWMutex
	factories map[string]Factory
}

// DefaultRegistry is the process-wide registry; populated by main.go at boot.
var DefaultRegistry = &Registry{factories: make(map[string]Factory)}

// Register adds a factory for a channel type. Safe to call during init.
func (r *Registry) Register(channelType string, f Factory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[channelType] = f
}

// Build constructs a Channel from a stored user_channels row.
// Returns an error if no factory is registered for the given type.
func (r *Registry) Build(ctx context.Context, uc domain.UserChannel) (Channel, error) {
	r.mu.RLock()
	f, ok := r.factories[uc.Type]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("channel: no factory registered for type %q", uc.Type)
	}
	return f(ctx, uc)
}
