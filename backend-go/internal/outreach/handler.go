// Package outreach composes the sub-handlers (sender profile, contacts,
// channels, conversations, campaigns) into a single router mounted at
// /outreach. The compliance handler is wired separately by the server
// because it must be public (no auth).
package outreach

import (
	"github.com/go-chi/chi/v5"

	"github.com/batuhan/marketintel/internal/outreach/campaign"
	"github.com/batuhan/marketintel/internal/outreach/channel"
	"github.com/batuhan/marketintel/internal/outreach/compliance"
	"github.com/batuhan/marketintel/internal/outreach/contact"
	"github.com/batuhan/marketintel/internal/outreach/contactgroup"
	"github.com/batuhan/marketintel/internal/outreach/conversation"
	"github.com/batuhan/marketintel/internal/outreach/crm"
	"github.com/batuhan/marketintel/internal/outreach/events"
	"github.com/batuhan/marketintel/internal/outreach/group"
	"github.com/batuhan/marketintel/internal/outreach/sender"
	"github.com/batuhan/marketintel/internal/outreach/sequence"
	"github.com/batuhan/marketintel/internal/outreach/simulation"
)

type Handler struct {
	sender       *sender.Handler
	contact      *contact.Handler
	channel      *channel.Handler
	conversation *conversation.Handler
	campaign     *campaign.Handler
	sequence     *sequence.Handler
	group        *group.Handler
	contactGroup *contactgroup.Handler
	simulation   *simulation.Handler
	crm          *crm.Handler
	events       *events.Handler
	compliance   *compliance.Handler
}

func NewHandler(
	senderH *sender.Handler,
	contactH *contact.Handler,
	channelH *channel.Handler,
	convH *conversation.Handler,
	campaignH *campaign.Handler,
	sequenceH *sequence.Handler,
	groupH *group.Handler,
	contactGroupH *contactgroup.Handler,
	simulationH *simulation.Handler,
	crmH *crm.Handler,
	eventsH *events.Handler,
	complianceH *compliance.Handler,
) *Handler {
	// Wire nested notes routes onto the contact handler before its Routes()
	// is materialised by the server.
	if contactH != nil && crmH != nil {
		contactH.SetNestedRoutes(crmH.ContactNotesRoutes())
	}
	return &Handler{
		sender: senderH, contact: contactH, channel: channelH,
		conversation: convH, campaign: campaignH, sequence: sequenceH,
		group: groupH, contactGroup: contactGroupH, simulation: simulationH,
		crm: crmH, events: eventsH, compliance: complianceH,
	}
}

// ChannelHandler returns the inner channel handler so the server can register
// its Gmail OAuth callback as a public (unauthenticated) endpoint.
func (h *Handler) ChannelHandler() *channel.Handler { return h.channel }

// ComplianceHandler returns the inner compliance handler so the server can
// register the public /unsubscribe routes outside the auth middleware.
func (h *Handler) ComplianceHandler() *compliance.Handler { return h.compliance }

// Routes returns the authenticated /outreach/* router.
func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Mount("/sender-profile", h.sender.Routes())
	r.Mount("/contacts", h.contact.Routes())
	r.Mount("/channels", h.channel.Routes())
	r.Mount("/conversations", h.conversation.Routes())
	if h.campaign != nil {
		r.Mount("/campaigns", h.campaign.Routes())
	}
	if h.sequence != nil {
		r.Mount("/sequences", h.sequence.Routes())
	}
	if h.group != nil {
		r.Mount("/groups", h.group.Routes())
	}
	if h.contactGroup != nil {
		r.Mount("/contact-groups", h.contactGroup.Routes())
	}
	if h.simulation != nil {
		r.Mount("/simulations", h.simulation.Routes())
	}
	if h.crm != nil {
		r.Mount("/tasks", h.crm.TaskRoutes())
	}
	if h.events != nil {
		r.Mount("/events", h.events.Routes())
	}
	return r
}
