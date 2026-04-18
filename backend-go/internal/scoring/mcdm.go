package scoring

import (
	"math"
	"sort"
)

// Criteria definitions: (name, category, type).
// type: +1 = benefit (higher is better), -1 = cost (lower is better).
var criteria = []struct {
	Name     string
	Category string
	Type     float64 // +1 or -1
}{
	// Opportunity (0-3)
	{"import_value_usd", "opportunity", 1},
	{"import_growth_pct", "opportunity", 1},
	{"gdp_per_capita", "opportunity", 1},
	{"market_size_indicator", "opportunity", 1},
	// Reliability (4-7)
	{"political_stability", "reliability", 1},
	{"ease_of_doing_business", "reliability", 1},
	{"payment_reliability", "reliability", 1},
	{"currency_stability", "reliability", 1},
	// Accessibility (8-11)
	{"geographic_distance_km", "accessibility", -1}, // lower is better
	{"tariff_rate_pct", "accessibility", -1},         // lower is better
	{"logistics_performance", "accessibility", 1},
	{"trade_agreement_score", "accessibility", 1},
}

var (
	opportunityIdx  = []int{0, 1, 2, 3}
	reliabilityIdx  = []int{4, 5, 6, 7}
	accessibilityIdx = []int{8, 9, 10, 11}
)

// CriteriaNames returns all criterion names in order.
func CriteriaNames() []string {
	names := make([]string, len(criteria))
	for i, c := range criteria {
		names[i] = c.Name
	}
	return names
}

// RankingResult is the output of the MCDM ranking.
type RankingResult struct {
	CountryCode        string             `json:"country_code"`
	TOPSISScore        float64            `json:"topsis_score"`
	SAWScore           float64            `json:"saw_score"`
	OpportunityScore   float64            `json:"opportunity_score"`
	ReliabilityScore   float64            `json:"reliability_score"`
	AccessibilityScore float64            `json:"accessibility_score"`
	Weights            map[string]float64 `json:"weights"`
}

// RankMarkets ranks markets using TOPSIS + WSM (SAW).
// criteriaData: slice of maps, each with "country_code" and criterion values.
func RankMarkets(criteriaData []map[string]any) []RankingResult {
	n := len(criteriaData)
	if n == 0 {
		return nil
	}
	if n == 1 {
		return []RankingResult{{
			CountryCode:        getString(criteriaData[0], "country_code"),
			TOPSISScore:        1.0,
			SAWScore:           1.0,
			OpportunityScore:   1.0,
			ReliabilityScore:   1.0,
			AccessibilityScore: 1.0,
		}}
	}

	m := len(criteria)

	// Build decision matrix [n alternatives x m criteria]
	matrix := make([][]float64, n)
	for i, data := range criteriaData {
		matrix[i] = make([]float64, m)
		for j, c := range criteria {
			matrix[i][j] = getFloat(data, c.Name)
		}
	}

	// Handle zero-variance columns
	for j := 0; j < m; j++ {
		stddev := colStdDev(matrix, j, n)
		if stddev == 0 {
			for i := 0; i < n; i++ {
				matrix[i][j] += 0.001 * float64(i+1)
			}
		}
	}

	// Compute weights using entropy method
	weights := entropyWeights(matrix, n, m)

	// TOPSIS
	topsisScores := topsis(matrix, weights, n, m)

	// WSM / SAW
	sawScores := wsm(matrix, weights, n, m)

	// Normalize to 0-1
	normalizeMax(topsisScores)
	normalizeMax(sawScores)

	// Build results with category breakdowns
	results := make([]RankingResult, n)
	for i, data := range criteriaData {
		wMap := make(map[string]float64, m)
		for j, c := range criteria {
			wMap[c.Name] = math.Round(weights[j]*10000) / 10000
		}

		results[i] = RankingResult{
			CountryCode:        getString(data, "country_code"),
			TOPSISScore:        round4(topsisScores[i]),
			SAWScore:           round4(sawScores[i]),
			OpportunityScore:   categoryScore(matrix[i], weights, opportunityIdx),
			ReliabilityScore:   categoryScore(matrix[i], weights, reliabilityIdx),
			AccessibilityScore: categoryScore(matrix[i], weights, accessibilityIdx),
			Weights:            wMap,
		}
	}

	// Normalize category scores across alternatives
	normalizeCategoryScores(results, "opportunity")
	normalizeCategoryScores(results, "reliability")
	normalizeCategoryScores(results, "accessibility")

	// Sort by TOPSIS descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].TOPSISScore > results[j].TOPSISScore
	})

	return results
}

// --- TOPSIS ---

func topsis(matrix [][]float64, weights []float64, n, m int) []float64 {
	// Step 1: Normalize matrix (vector normalization)
	norm := make([][]float64, n)
	for i := range norm {
		norm[i] = make([]float64, m)
	}

	for j := 0; j < m; j++ {
		var sumSq float64
		for i := 0; i < n; i++ {
			sumSq += matrix[i][j] * matrix[i][j]
		}
		denom := math.Sqrt(sumSq)
		if denom == 0 {
			denom = 1
		}
		for i := 0; i < n; i++ {
			norm[i][j] = (matrix[i][j] / denom) * weights[j]
		}
	}

	// Step 2: Find ideal best and worst
	idealBest := make([]float64, m)
	idealWorst := make([]float64, m)
	for j := 0; j < m; j++ {
		minV, maxV := norm[0][j], norm[0][j]
		for i := 1; i < n; i++ {
			if norm[i][j] < minV {
				minV = norm[i][j]
			}
			if norm[i][j] > maxV {
				maxV = norm[i][j]
			}
		}
		if criteria[j].Type > 0 { // benefit
			idealBest[j] = maxV
			idealWorst[j] = minV
		} else { // cost
			idealBest[j] = minV
			idealWorst[j] = maxV
		}
	}

	// Step 3: Calculate distances and closeness coefficient
	scores := make([]float64, n)
	for i := 0; i < n; i++ {
		var dBest, dWorst float64
		for j := 0; j < m; j++ {
			dBest += (norm[i][j] - idealBest[j]) * (norm[i][j] - idealBest[j])
			dWorst += (norm[i][j] - idealWorst[j]) * (norm[i][j] - idealWorst[j])
		}
		dBest = math.Sqrt(dBest)
		dWorst = math.Sqrt(dWorst)
		if dBest+dWorst == 0 {
			scores[i] = 0
		} else {
			scores[i] = dWorst / (dBest + dWorst)
		}
	}
	return scores
}

// --- WSM (Weighted Sum Model / SAW) ---

func wsm(matrix [][]float64, weights []float64, n, m int) []float64 {
	// Normalize: benefit → val/max, cost → min/val
	colMin := make([]float64, m)
	colMax := make([]float64, m)
	for j := 0; j < m; j++ {
		colMin[j] = matrix[0][j]
		colMax[j] = matrix[0][j]
		for i := 1; i < n; i++ {
			if matrix[i][j] < colMin[j] {
				colMin[j] = matrix[i][j]
			}
			if matrix[i][j] > colMax[j] {
				colMax[j] = matrix[i][j]
			}
		}
	}

	scores := make([]float64, n)
	for i := 0; i < n; i++ {
		for j := 0; j < m; j++ {
			var normVal float64
			if criteria[j].Type > 0 { // benefit
				if colMax[j] != 0 {
					normVal = matrix[i][j] / colMax[j]
				}
			} else { // cost
				if matrix[i][j] != 0 {
					normVal = colMin[j] / matrix[i][j]
				}
			}
			scores[i] += normVal * weights[j]
		}
	}
	return scores
}

// --- Entropy weights ---

func entropyWeights(matrix [][]float64, n, m int) []float64 {
	// Normalize each column to sum to 1
	pij := make([][]float64, n)
	for i := range pij {
		pij[i] = make([]float64, m)
	}

	for j := 0; j < m; j++ {
		var colSum float64
		for i := 0; i < n; i++ {
			colSum += math.Abs(matrix[i][j])
		}
		if colSum == 0 {
			colSum = 1
		}
		for i := 0; i < n; i++ {
			pij[i][j] = math.Abs(matrix[i][j]) / colSum
		}
	}

	k := 1.0 / math.Log(float64(n))
	entropy := make([]float64, m)
	for j := 0; j < m; j++ {
		var ej float64
		for i := 0; i < n; i++ {
			if pij[i][j] > 0 {
				ej -= pij[i][j] * math.Log(pij[i][j])
			}
		}
		entropy[j] = k * ej
	}

	// Divergence = 1 - entropy
	var totalDiv float64
	divergence := make([]float64, m)
	for j := 0; j < m; j++ {
		divergence[j] = 1 - entropy[j]
		if divergence[j] < 0 {
			divergence[j] = 0
		}
		totalDiv += divergence[j]
	}

	weights := make([]float64, m)
	if totalDiv == 0 {
		// Equal weights fallback
		for j := 0; j < m; j++ {
			weights[j] = 1.0 / float64(m)
		}
	} else {
		for j := 0; j < m; j++ {
			weights[j] = divergence[j] / totalDiv
		}
	}
	return weights
}

// --- Helpers ---

func colStdDev(matrix [][]float64, col, n int) float64 {
	var sum float64
	for i := 0; i < n; i++ {
		sum += matrix[i][col]
	}
	mean := sum / float64(n)
	var variance float64
	for i := 0; i < n; i++ {
		d := matrix[i][col] - mean
		variance += d * d
	}
	return math.Sqrt(variance / float64(n))
}

func categoryScore(row []float64, weights []float64, indices []int) float64 {
	var weightedSum, weightSum float64
	for _, idx := range indices {
		weightedSum += row[idx] * weights[idx]
		weightSum += weights[idx]
	}
	if weightSum == 0 {
		return 0
	}
	return weightedSum / weightSum
}

func normalizeCategoryScores(results []RankingResult, category string) {
	var maxVal float64
	for _, r := range results {
		var v float64
		switch category {
		case "opportunity":
			v = r.OpportunityScore
		case "reliability":
			v = r.ReliabilityScore
		case "accessibility":
			v = r.AccessibilityScore
		}
		if v > maxVal {
			maxVal = v
		}
	}
	if maxVal == 0 {
		return
	}
	for i := range results {
		switch category {
		case "opportunity":
			results[i].OpportunityScore = round4(results[i].OpportunityScore / maxVal)
		case "reliability":
			results[i].ReliabilityScore = round4(results[i].ReliabilityScore / maxVal)
		case "accessibility":
			results[i].AccessibilityScore = round4(results[i].AccessibilityScore / maxVal)
		}
	}
}

func normalizeMax(scores []float64) {
	var maxVal float64
	for _, s := range scores {
		if s > maxVal {
			maxVal = s
		}
	}
	if maxVal == 0 {
		return
	}
	for i := range scores {
		scores[i] /= maxVal
	}
}

func round4(v float64) float64 {
	return math.Round(v*10000) / 10000
}

func getFloat(m map[string]any, key string) float64 {
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	case int64:
		return float64(val)
	default:
		return 0
	}
}

func getString(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}
