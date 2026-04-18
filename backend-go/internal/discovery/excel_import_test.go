package discovery

import (
	"testing"
)

func TestNormalizeCountry(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"TR", "TURKEY"},
		{"tr", "TURKEY"},
		{"TURKIYE", "TURKEY"},
		{"Türkiye", "TURKEY"},
		{"US", "UNITED STATES"},
		{"USA", "UNITED STATES"},
		{"UNITED STATES OF AMERICA", "UNITED STATES"},
		{"DE", "GERMANY"},
		{"GB", "UNITED KINGDOM"},
		{"UK", "UNITED KINGDOM"},
		{"CN", "CHINA"},
		{"BRAZIL", "BRAZIL"},       // unmapped, returns as-is uppercased
		{"  FR  ", "FRANCE"},        // whitespace trimmed
		{"", ""},
	}

	for _, tt := range tests {
		got := normalizeCountry(tt.input)
		if got != tt.expected {
			t.Errorf("normalizeCountry(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestCoalesce(t *testing.T) {
	if v := coalesce("", "", "c"); v != "c" {
		t.Errorf("expected 'c', got %q", v)
	}
	if v := coalesce("a", "b"); v != "a" {
		t.Errorf("expected 'a', got %q", v)
	}
	if v := coalesce("", ""); v != "" {
		t.Errorf("expected empty, got %q", v)
	}
}

func TestContainsStr(t *testing.T) {
	slice := []string{"Apple", "BANANA", "cherry"}
	if !containsStr(slice, "apple") {
		t.Error("should find 'apple' case-insensitively")
	}
	if !containsStr(slice, "CHERRY") {
		t.Error("should find 'CHERRY' case-insensitively")
	}
	if containsStr(slice, "grape") {
		t.Error("should not find 'grape'")
	}
	if containsStr(nil, "any") {
		t.Error("should not find in nil slice")
	}
}

func TestParseFloat(t *testing.T) {
	tests := []struct {
		input    string
		expected float64
	}{
		{"123.45", 123.45},
		{"0", 0},
		{"", 0},
		{"None", 0},
		{"  42.5  ", 42.5},
		{"1000", 1000},
	}

	for _, tt := range tests {
		got := parseFloat(tt.input)
		if got != tt.expected {
			t.Errorf("parseFloat(%q) = %f, want %f", tt.input, got, tt.expected)
		}
	}
}

func TestBuildColumnIndex(t *testing.T) {
	header := []string{"Arrival Date", "Consignee Name", "HS Code", "Total Price"}
	idx := buildColumnIndex(header)

	row := []string{"2024-01-15", "ACME Corp", "2515", "1234.56"}

	if v := idx.get(row, "Arrival Date"); v != "2024-01-15" {
		t.Errorf("expected '2024-01-15', got %q", v)
	}
	if v := idx.get(row, "Consignee Name"); v != "ACME Corp" {
		t.Errorf("expected 'ACME Corp', got %q", v)
	}
	if v := idx.get(row, "HS Code"); v != "2515" {
		t.Errorf("expected '2515', got %q", v)
	}

	// Case insensitive lookup
	if v := idx.get(row, "arrival date"); v != "2024-01-15" {
		t.Errorf("case-insensitive lookup failed, got %q", v)
	}

	// Missing column
	if v := idx.get(row, "Nonexistent"); v != "" {
		t.Errorf("expected empty for missing column, got %q", v)
	}

	// Fallback names
	if v := idx.get(row, "Missing", "Consignee Name"); v != "ACME Corp" {
		t.Errorf("fallback name lookup failed, got %q", v)
	}
}

func TestAggregateByConsignee(t *testing.T) {
	records := []ShipmentRecord{
		{
			ConsigneeStdName: "ACME CORP",
			ConsigneeCity:    "Berlin",
			ConsigneeCountry: "Germany",
			ShipperName:      "Turkish Exports Ltd",
			ShipperStdName:   "Turkish Exports Ltd",
			ShipperCountry:   "TURKEY",
			HSCode:           "2515",
			GrossWeightKG:    500,
			TotalPrice:       10000,
			ArrivalDate:      "2024-01-15",
		},
		{
			ConsigneeStdName: "ACME CORP",
			ConsigneeCity:    "Berlin",
			ConsigneeCountry: "Germany",
			ShipperName:      "China Trading Co",
			ShipperStdName:   "China Trading Co",
			ShipperCountry:   "CHINA",
			HSCode:           "2516",
			GrossWeightKG:    300,
			TotalPrice:       8000,
			ArrivalDate:      "2024-02-20",
		},
		{
			ConsigneeStdName: "BETA GMBH",
			ConsigneeCity:    "Munich",
			ConsigneeCountry: "Germany",
			ShipperName:      "Italian Marble SpA",
			ShipperStdName:   "Italian Marble SpA",
			ShipperCountry:   "ITALY",
			HSCode:           "2515",
			GrossWeightKG:    1000,
			TotalPrice:       25000,
			ArrivalDate:      "2024-03-10",
		},
	}

	result := aggregateByConsignee(records, "TURKEY")

	if len(result) != 2 {
		t.Fatalf("expected 2 importers, got %d", len(result))
	}

	// Should be sorted by transaction count descending
	// ACME has 2 transactions, BETA has 1
	if result[0].CompanyName != "ACME CORP" {
		t.Errorf("expected ACME CORP first (most transactions), got %s", result[0].CompanyName)
	}

	acme := result[0]
	if acme.TransactionCount != 2 {
		t.Errorf("ACME transaction count: expected 2, got %d", acme.TransactionCount)
	}
	if acme.TotalWeightKG != 800 {
		t.Errorf("ACME total weight: expected 800, got %f", acme.TotalWeightKG)
	}
	if acme.TotalValueUSD != 18000 {
		t.Errorf("ACME total value: expected 18000, got %f", acme.TotalValueUSD)
	}
	if len(acme.HSCodes) != 2 {
		t.Errorf("ACME should have 2 HS codes, got %d", len(acme.HSCodes))
	}
	if len(acme.Suppliers) != 2 {
		t.Errorf("ACME should have 2 suppliers, got %d", len(acme.Suppliers))
	}
	if !acme.BuysFromHome {
		t.Error("ACME should BuysFromHome (has Turkish supplier)")
	}
	if acme.TrustTier != "confirmed_buyer" {
		t.Errorf("ACME trust tier: expected confirmed_buyer, got %s", acme.TrustTier)
	}
	if acme.LastShipmentDate != "2024-02-20" {
		t.Errorf("ACME last shipment: expected 2024-02-20, got %s", acme.LastShipmentDate)
	}

	beta := result[1]
	if beta.BuysFromHome {
		t.Error("BETA should not BuysFromHome (no Turkish supplier)")
	}
	if beta.TrustTier != "confirmed_importer" {
		t.Errorf("BETA trust tier: expected confirmed_importer, got %s", beta.TrustTier)
	}
}

func TestAggregateByConsigneeEmpty(t *testing.T) {
	result := aggregateByConsignee(nil, "TURKEY")
	if len(result) != 0 {
		t.Errorf("expected 0 importers for nil input, got %d", len(result))
	}
}

func TestShipmentContextForAI(t *testing.T) {
	// Test with nil
	if v := shipmentContextForAI(nil); v != "" {
		t.Errorf("expected empty for nil, got %q", v)
	}

	// Test with valid shipment data (flat JSON — stored directly in shipment_data column)
	data := []byte(`{"transaction_count":5,"total_weight_kg":2500,"total_value_usd":50000,"last_shipment_date":"2024-03-01","products":["MARBLE BLOCKS"],"suppliers":[{"name":"TURK MARBLE","country":"TURKEY","city":"Afyon"}],"buys_from_home":true,"trust_tier":"confirmed_buyer"}`)
	ctx := shipmentContextForAI(data)

	if ctx == "" {
		t.Fatal("expected non-empty context")
	}
	if !contains(ctx, "confirmed_buyer") {
		t.Error("context should contain trust tier")
	}
	if !contains(ctx, "already imports from your country") {
		t.Error("context should indicate buys from home")
	}
	if !contains(ctx, "5 shipments") {
		t.Error("context should contain transaction count")
	}
	if !contains(ctx, "MARBLE BLOCKS") {
		t.Error("context should contain products")
	}
	if !contains(ctx, "TURK MARBLE") {
		t.Error("context should contain supplier name")
	}
}

func TestShipmentContextForAINoShipmentData(t *testing.T) {
	// Empty JSON object — all fields zero. Function returns a minimal string.
	data := []byte(`{}`)
	got := shipmentContextForAI(data)
	if got == "" {
		t.Errorf("expected some context even for empty data, got empty")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
