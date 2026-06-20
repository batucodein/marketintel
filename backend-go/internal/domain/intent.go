package domain

// Intent tags are a controlled vocabulary the classifier may emit for an inbound
// reply. They are the single source of truth: the prompt lists them, the parser
// validates against them (anything off-list is dropped — no hallucinated
// intent), and the engine routes on the compliance/routing subset.

// Buying signals.
const (
	TagPriceRequested        = "price_requested"
	TagInfoRequested         = "info_requested"
	TagSampleRequested       = "sample_requested"
	TagMeetingRequested      = "meeting_requested"
	TagMOQQuestion           = "moq_question"
	TagShippingQuestion      = "shipping_question"
	TagCertificationQuestion = "certification_question"
	TagReadyToOrder          = "ready_to_order"
)

// Objections.
const (
	TagPriceObjection  = "price_objection"
	TagTimingObjection = "timing_objection"
	TagHasSupplier     = "has_supplier"
	TagNotInterested   = "not_interested"
)

// Routing / compliance — these override the sentiment branch in the engine.
const (
	TagUnsubscribe  = "unsubscribe"
	TagOutOfOffice  = "out_of_office"
	TagWrongContact = "wrong_contact"
	TagReferral     = "referral"
)

// IntentVocabulary is the whitelist of valid tags. The classifier parser drops
// any tag not present here.
var IntentVocabulary = map[string]bool{
	TagPriceRequested: true, TagInfoRequested: true, TagSampleRequested: true,
	TagMeetingRequested: true, TagMOQQuestion: true, TagShippingQuestion: true,
	TagCertificationQuestion: true, TagReadyToOrder: true,
	TagPriceObjection: true, TagTimingObjection: true, TagHasSupplier: true,
	TagNotInterested: true,
	TagUnsubscribe:   true, TagOutOfOffice: true, TagWrongContact: true, TagReferral: true,
}

// IsValidIntentTag reports whether t is in the controlled vocabulary.
func IsValidIntentTag(t string) bool { return IntentVocabulary[t] }

// IsComplianceTag is the subset that must suppress contact (legal).
func IsComplianceTag(t string) bool { return t == TagUnsubscribe }

// IsRoutingTag is the subset that overrides the sentiment branch in the engine.
func IsRoutingTag(t string) bool {
	switch t {
	case TagUnsubscribe, TagOutOfOffice, TagWrongContact:
		return true
	}
	return false
}
