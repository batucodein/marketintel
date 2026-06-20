package simulation

// Persona is a simulated buyer archetype. RepliesAt makes the scenario
// deterministic (when they reply); the AI generates realistic content; the
// IntentPrompt steers tone/intent; ExpectedSentiment is used to score the
// sentiment classifier's accuracy.
type Persona struct {
	Key               string
	Label             string
	IntentPrompt      string
	ExpectedSentiment string // positive | neutral | negative | "" (no reply)
	NoEmail           bool
	repliesAt         map[int]bool // touch index → replies (0 = initial cold email)
	unsubscribes      bool
}

func (p Persona) RepliesAt(touch int) bool { return p.repliesAt[touch] }

// Personas is the catalog exposed to the UI.
var Personas = []Persona{
	{
		Key: "eager_buyer", Label: "Eager buyer (asks pricing)",
		IntentPrompt:      "You are genuinely interested. The product fits a real need. You reply warmly and ask for pricing, a sample, or a quick call. You are ready to engage.",
		ExpectedSentiment: "positive",
		repliesAt:         map[int]bool{0: true},
	},
	{
		Key: "price_negotiator", Label: "Price negotiator",
		IntentPrompt:      "You are interested but cost-focused. You reply asking about price, minimums, lead times, and push back that you already have a supplier. Conditional interest, not a clear yes.",
		ExpectedSentiment: "neutral",
		repliesAt:         map[int]bool{0: true, 1: true},
	},
	{
		Key: "ghoster", Label: "Ghoster (never replies)",
		IntentPrompt:      "You never reply. (This persona is silent.)",
		ExpectedSentiment: "",
		repliesAt:         map[int]bool{},
	},
	{
		Key: "slow_warmer", Label: "Slow warmer (replies on 3rd touch)",
		IntentPrompt:      "You ignored the first messages because you were busy, but the persistence + value finally caught your attention. You now reply positively and ask to learn more.",
		ExpectedSentiment: "positive",
		repliesAt:         map[int]bool{2: true},
	},
	{
		Key: "not_interested", Label: "Not interested",
		IntentPrompt:      "You are not interested. You reply politely but clearly declining — wrong timing or wrong fit. You do not want follow-ups.",
		ExpectedSentiment: "negative",
		repliesAt:         map[int]bool{0: true},
	},
	{
		Key: "unsubscriber", Label: "Unsubscriber (remove me)",
		IntentPrompt:      "You are annoyed by cold outreach. You reply tersely asking to be removed / to stop emailing you.",
		ExpectedSentiment: "negative",
		repliesAt:         map[int]bool{0: true},
		unsubscribes:      true,
	},
	{
		Key: "wrong_contact", Label: "Wrong contact",
		IntentPrompt:      "You are not the right person. You reply that purchasing is handled by a colleague and they should contact someone else (invent a name/role).",
		ExpectedSentiment: "neutral",
		repliesAt:         map[int]bool{0: true},
	},
	{
		Key: "out_of_office", Label: "Out of office (auto-reply)",
		IntentPrompt:      "Your mailbox sends an automatic out-of-office reply: you are away for two weeks with no email access, will respond on return, and give an alternate contact for urgent matters. No personal engagement.",
		ExpectedSentiment: "neutral",
		repliesAt:         map[int]bool{0: true},
	},
	{
		Key: "no_email", Label: "No email on file (skipped)",
		IntentPrompt:      "",
		ExpectedSentiment: "",
		NoEmail:           true,
		repliesAt:         map[int]bool{},
	},
}

func personaByKey(key string) (Persona, bool) {
	for _, p := range Personas {
		if p.Key == key {
			return p, true
		}
	}
	return Persona{}, false
}
