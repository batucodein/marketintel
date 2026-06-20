package prompts

import "testing"

func TestValidateOutreachBody_Clean(t *testing.T) {
	body := `Hi there, I noticed your recent imports of Carrara marble from Italy and thought our Turkish quarry might be a useful alternate supplier. We work with similar specifications and can hold our pricing for 30 days. I've attached our catalog for your reference. Would a 15-minute call next week make sense to discuss specs and lead times?`
	v := ValidateOutreachBody(body, true, DraftModeCold)
	if len(v) != 0 {
		t.Fatalf("expected no violations on clean body, got %+v", v)
	}
}

func TestValidateOutreachBody_CatalogHedge(t *testing.T) {
	cases := []string{
		"Would it make sense for me to send our catalog?",
		"Should I send you our catalog?",
		"Let me know if you'd like me to share our catalog.",
		"I can send you our catalog if useful.",
		"Happy to forward our catalog if you're interested.",
	}
	for _, body := range cases {
		v := ValidateOutreachBody(body, true, DraftModeCold)
		hit := false
		for _, gv := range v {
			if gv.Kind == "catalog_question" {
				hit = true
				break
			}
		}
		if !hit {
			t.Errorf("expected catalog_question violation for %q, got %+v", body, v)
		}
	}
}

func TestValidateOutreachBody_NoCatalogNoCheck(t *testing.T) {
	// When the campaign has no catalog attached, catalog phrases are
	// allowed (the recipient is being offered one in a normal way).
	body := "Let me know if you'd like our catalog."
	v := ValidateOutreachBody(body, false, DraftModeCold)
	for _, gv := range v {
		if gv.Kind == "catalog_question" {
			t.Fatalf("did not expect catalog_question when hasCatalog=false, got %+v", v)
		}
	}
}

func TestValidateOutreachBody_ForbiddenPhrases(t *testing.T) {
	cases := map[string]string{
		"hope this email finds you well": "I hope this email finds you well and that things are going great.",
		"i came across your company":     "I came across your company while researching importers in Spain.",
		"feel free to":                   "Feel free to reach out anytime.",
	}
	for phrase, body := range cases {
		v := ValidateOutreachBody(body, false, DraftModeCold)
		found := false
		for _, gv := range v {
			if gv.Kind == "forbidden_phrase" && gv.Detail == phrase {
				found = true
			}
		}
		if !found {
			t.Errorf("expected forbidden_phrase %q for body %q, got %+v", phrase, body, v)
		}
	}
}

func TestValidateOutreachBody_CircleBackAllowedInFollowup(t *testing.T) {
	body := "Just wanted to circle back on the Carrara samples we discussed last month."
	v := ValidateOutreachBody(body, false, DraftModeFollowup)
	for _, gv := range v {
		if gv.Kind == "forbidden_phrase" && (gv.Detail == "circle back" || gv.Detail == "touch base") {
			t.Errorf("circle back should be allowed in followup mode, got %+v", v)
		}
	}

	// But in cold mode, circle back IS forbidden.
	v = ValidateOutreachBody(body, false, DraftModeCold)
	caught := false
	for _, gv := range v {
		if gv.Kind == "forbidden_phrase" && gv.Detail == "circle back" {
			caught = true
		}
	}
	if !caught {
		t.Errorf("circle back should be forbidden in cold mode, got %+v", v)
	}
}

func TestValidateOutreachBody_ExclamationColdOnly(t *testing.T) {
	body := "Great to meet you! Let's set up time soon."
	if v := ValidateOutreachBody(body, false, DraftModeCold); !hasKind(v, "exclamation_in_cold") {
		t.Errorf("expected exclamation_in_cold for cold draft, got %+v", v)
	}
	if v := ValidateOutreachBody(body, false, DraftModeReply); hasKind(v, "exclamation_in_cold") {
		t.Errorf("exclamation should be allowed in reply, got %+v", v)
	}
}

func TestValidateOutreachBody_LengthCap(t *testing.T) {
	// 200 words — well over the 140 cold cap.
	long := ""
	for i := 0; i < 200; i++ {
		long += "word "
	}
	if v := ValidateOutreachBody(long, false, DraftModeCold); !hasKind(v, "length_cap") {
		t.Errorf("expected length_cap violation, got %+v", v)
	}
}

func hasKind(vs []GuardViolation, kind string) bool {
	for _, v := range vs {
		if v.Kind == kind {
			return true
		}
	}
	return false
}
