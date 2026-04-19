// Package outreach composes the sub-handlers (sender profile, contacts,
// channels, conversations) into a single router mounted at /outreach.
package outreach

import (
	"github.com/go-chi/chi/v5"

	"github.com/batuhan/marketintel/internal/outreach/channel"
	"github.com/batuhan/marketintel/internal/outreach/contact"
	"github.com/batuhan/marketintel/internal/outreach/conversation"
	"github.com/batuhan/marketintel/internal/outreach/sender"
)

type Handler struct {
	sender       *sender.Handler
	contact      *contact.Handler
	channel      *channel.Handler
	conversation *conversation.Handler
}

func NewHandler(senderH *sender.Handler, contactH *contact.Handler, channelH *channel.Handler, convH *conversation.Handler) *Handler {
	return &Handler{sender: senderH, contact: contactH, channel: channelH, conversation: convH}
}

// ChannelHandler returns the inner channel handler so the server can register
// its Gmail OAuth callback as a public (unauthenticated) endpoint.
func (h *Handler) ChannelHandler() *channel.Handler { return h.channel }

// Routes returns the authenticated /outreach/* router.
func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Mount("/sender-profile", h.sender.Routes())
	r.Mount("/contacts", h.contact.Routes())
	r.Mount("/channels", h.channel.Routes())
	r.Mount("/conversations", h.conversation.Routes())
	return r
}
