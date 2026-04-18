package scoring

import (
	"math"
	"testing"
)

func TestRankMarketsEmpty(t *testing.T) {
	result := RankMarkets(nil)
	if result != nil {
		t.Errorf("expected nil for empty input, got %v", result)
	}
}

func TestRankMarketsSingleCountry(t *testing.T) {
	data := []map[string]any{
		{"country_code": "DE", "import_value_usd": 1000000.0},
	}
	results := RankMarkets(data)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].CountryCode != "DE" {
		t.Errorf("expected DE, got %s", results[0].CountryCode)
	}
	if results[0].TOPSISScore != 1.0 {
		t.Errorf("single country should have TOPSIS=1.0, got %f", results[0].TOPSISScore)
	}
}

func TestRankMarketsMultipleCountries(t *testing.T) {
	data := []map[string]any{
		{
			"country_code":           "DE",
			"import_value_usd":       5000000.0,
			"import_growth_pct":      8.0,
			"gdp_per_capita":         48000.0,
			"market_size_indicator":  83.0,
			"political_stability":    0.8,
			"ease_of_doing_business": 80.0,
			"payment_reliability":    0.9,
			"currency_stability":     0.95,
			"geographic_distance_km": 2000.0,
			"tariff_rate_pct":        0.0,
			"logistics_performance":  4.2,
			"trade_agreement_score":  1.0,
		},
		{
			"country_code":           "NG",
			"import_value_usd":       500000.0,
			"import_growth_pct":      3.0,
			"gdp_per_capita":         2000.0,
			"market_size_indicator":  200.0,
			"political_stability":    0.3,
			"ease_of_doing_business": 40.0,
			"payment_reliability":    0.4,
			"currency_stability":     0.3,
			"geographic_distance_km": 5000.0,
			"tariff_rate_pct":        15.0,
			"logistics_performance":  2.5,
			"trade_agreement_score":  0.0,
		},
		{
			"country_code":           "US",
			"import_value_usd":       10000000.0,
			"import_growth_pct":      5.0,
			"gdp_per_capita":         65000.0,
			"market_size_indicator":  330.0,
			"political_stability":    0.7,
			"ease_of_doing_business": 85.0,
			"payment_reliability":    0.85,
			"currency_stability":     0.9,
			"geographic_distance_km": 9000.0,
			"tariff_rate_pct":        5.0,
			"logistics_performance":  3.9,
			"trade_agreement_score":  0.5,
		},
	}

	results := RankMarkets(data)
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// Should be sorted by TOPSIS descending
	for i := 1; i < len(results); i++ {
		if results[i].TOPSISScore > results[i-1].TOPSISScore {
			t.Errorf("results not sorted: [%d]=%f > [%d]=%f",
				i, results[i].TOPSISScore, i-1, results[i-1].TOPSISScore)
		}
	}

	// All scores should be in [0, 1]
	for _, r := range results {
		if r.TOPSISScore < 0 || r.TOPSISScore > 1 {
			t.Errorf("%s TOPSIS score out of range: %f", r.CountryCode, r.TOPSISScore)
		}
		if r.SAWScore < 0 || r.SAWScore > 1 {
			t.Errorf("%s SAW score out of range: %f", r.CountryCode, r.SAWScore)
		}
		if r.OpportunityScore < 0 || r.OpportunityScore > 1 {
			t.Errorf("%s Opportunity score out of range: %f", r.CountryCode, r.OpportunityScore)
		}
	}

	// Top-ranked should have TOPSIS=1.0 (normalized)
	if results[0].TOPSISScore != 1.0 {
		t.Errorf("top-ranked should be 1.0, got %f", results[0].TOPSISScore)
	}

	// Weights should be present and sum to ~1.0
	if len(results[0].Weights) != 12 {
		t.Errorf("expected 12 weights, got %d", len(results[0].Weights))
	}
	var weightSum float64
	for _, w := range results[0].Weights {
		weightSum += w
	}
	if math.Abs(weightSum-1.0) > 0.01 {
		t.Errorf("weights should sum to ~1.0, got %f", weightSum)
	}
}

func TestEntropyWeightsEqualColumns(t *testing.T) {
	// When all columns have same variance, weights should be equal
	matrix := [][]float64{
		{1, 1, 1},
		{2, 2, 2},
		{3, 3, 3},
	}
	weights := entropyWeights(matrix, 3, 3)
	for i, w := range weights {
		if math.Abs(w-1.0/3.0) > 0.01 {
			t.Errorf("weight[%d] should be ~0.333, got %f", i, w)
		}
	}
}

func TestEntropyWeightsHighVarianceGetsMore(t *testing.T) {
	// Column 0 has much higher variance → should get higher weight
	matrix := [][]float64{
		{100, 1, 1},
		{200, 2, 2},
		{1000, 3, 3},
	}
	weights := entropyWeights(matrix, 3, 3)
	if weights[0] <= weights[1] {
		t.Errorf("high-variance column should have higher weight: %f vs %f", weights[0], weights[1])
	}
}

func TestTOPSISBasic(t *testing.T) {
	// Two alternatives: one clearly better on all benefit criteria
	matrix := [][]float64{
		{10, 5},
		{5, 10},
	}
	weights := []float64{0.5, 0.5}

	// Both benefit criteria (use local topsis, criteria types don't apply here directly)
	scores := topsis(matrix, weights, 2, 2)

	// Both should be non-negative
	if scores[0] < 0 || scores[1] < 0 {
		t.Errorf("scores should be non-negative: %v", scores)
	}
}

func TestCriteriaNames(t *testing.T) {
	names := CriteriaNames()
	if len(names) != 12 {
		t.Errorf("expected 12 criteria, got %d", len(names))
	}
	if names[0] != "import_value_usd" {
		t.Errorf("first criterion should be import_value_usd, got %s", names[0])
	}
}

func TestGetFloat(t *testing.T) {
	m := map[string]any{
		"a": 1.5,
		"b": 42,
		"c": int64(100),
		"d": "not a number",
	}
	if v := getFloat(m, "a"); v != 1.5 {
		t.Errorf("expected 1.5, got %f", v)
	}
	if v := getFloat(m, "b"); v != 42.0 {
		t.Errorf("expected 42, got %f", v)
	}
	if v := getFloat(m, "c"); v != 100.0 {
		t.Errorf("expected 100, got %f", v)
	}
	if v := getFloat(m, "d"); v != 0 {
		t.Errorf("expected 0 for string, got %f", v)
	}
	if v := getFloat(m, "missing"); v != 0 {
		t.Errorf("expected 0 for missing, got %f", v)
	}
}
