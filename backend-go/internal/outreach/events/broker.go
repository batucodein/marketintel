// Package events is an in-memory pub/sub broker for outreach realtime
// updates. The poller publishes when a new inbound message lands; SSE
// subscribers (one per logged-in browser tab) receive the event and ask
// SWR to revalidate the affected conversation/inbox view.
//
// In-memory means events do NOT cross Cloud Run instances. With
// min-instances=0 and a single user this is fine; for multi-user we'd
// swap this implementation for Redis pub/sub without changing callers.
package events

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// Kind enumerates the event types subscribers may receive.
const (
	KindInbound            = "inbound"             // a new inbound message landed
	KindConversationRead   = "conversation_read"   // user marked a conversation read elsewhere
	KindCampaignProgress   = "campaign_progress"   // drafter/scheduler bumped a campaign counter
	KindSimulationProgress = "simulation_progress" // a simulation lead finished
)

// Event is the payload pushed to subscribers. UserID is used purely for
// routing (we never serialise it to clients) — every other field is
// rendered into the SSE message body.
type Event struct {
	Kind           string         `json:"kind"`
	UserID         uuid.UUID      `json:"-"`
	ConversationID *uuid.UUID     `json:"conversation_id,omitempty"`
	CampaignID     *uuid.UUID     `json:"campaign_id,omitempty"`
	At             time.Time      `json:"at"`
	Data           map[string]any `json:"data,omitempty"`
}

// Broker fans events out to per-user subscriber channels. Subscribers must
// drain quickly: a slow consumer will see events dropped (logged once).
type Broker struct {
	mu   sync.RWMutex
	subs map[uuid.UUID]map[string]chan Event
}

func NewBroker() *Broker {
	return &Broker{subs: make(map[uuid.UUID]map[string]chan Event)}
}

// Subscribe registers a subscriber for the given user. Returns the channel
// to read from and a cancel function to call on disconnect. Buffer size
// of 16 absorbs short bursts without blocking publishers.
func (b *Broker) Subscribe(userID uuid.UUID) (<-chan Event, func()) {
	id := uuid.New().String()
	ch := make(chan Event, 16)

	b.mu.Lock()
	if b.subs[userID] == nil {
		b.subs[userID] = make(map[string]chan Event)
	}
	b.subs[userID][id] = ch
	b.mu.Unlock()

	cancel := func() {
		b.mu.Lock()
		if peers, ok := b.subs[userID]; ok {
			delete(peers, id)
			if len(peers) == 0 {
				delete(b.subs, userID)
			}
		}
		b.mu.Unlock()
		close(ch)
	}
	return ch, cancel
}

// Publish broadcasts an event to every active subscriber of the user.
// Non-blocking — if a subscriber's buffer is full, the event is dropped
// for that subscriber only (fairness over reliability — clients revalidate
// via SWR cache anyway, so a missed event is recoverable on next render).
func (b *Broker) Publish(e Event) {
	if e.At.IsZero() {
		e.At = time.Now().UTC()
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, ch := range b.subs[e.UserID] {
		select {
		case ch <- e:
		default:
			// drop
		}
	}
}

// SubscriberCount returns the number of active subscriptions for the user.
// Used by the /outreach/events endpoint to surface a debug header.
func (b *Broker) SubscriberCount(userID uuid.UUID) int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subs[userID])
}
