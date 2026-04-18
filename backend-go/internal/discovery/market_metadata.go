package discovery

import (
	"sort"
	"strings"
	"time"
)

// ShipmentGroup is a subset of records sharing a single destination country.
type ShipmentGroup struct {
	DestinationCountry string // ISO-2 country code (e.g., "US")
	Records            []ShipmentRecord
}

// MarketMetadata is everything we derive deterministically from a ShipmentGroup.
// Zero AI, zero hallucination — pure counts, sorts, and percentiles.
type MarketMetadata struct {
	DestinationCountry string    // ISO-2
	DominantHSCode     string    // most-frequent 6-digit HS
	AllHSCodes         []string  // unique HS codes, sorted by frequency descending
	OriginCountry      string    // most-frequent shipper country (ISO-2)
	OriginShare        float64   // 0.0-1.0 share of shipments from dominant origin
	AllOriginCountries []string  // all unique shippers, sorted by frequency desc
	ShipmentFromDate   time.Time // 5th-percentile arrival date (zero if none parseable)
	ShipmentToDate     time.Time // 95th-percentile arrival date
	TopProductDescs    []string  // top 20 product descriptions, for AI grounding
	ImporterCount      int       // distinct consignee std names
	ShipmentCount      int       // total record count in group
}

// SplitByDestination groups shipment records by consignee country.
// Records with an unrecognized/missing country are grouped under "ZZ" (ISO-2 unassigned).
func SplitByDestination(records []ShipmentRecord) []ShipmentGroup {
	buckets := make(map[string][]ShipmentRecord)
	var order []string
	for _, r := range records {
		code := countryToISO2(r.ConsigneeCountry)
		if code == "" {
			code = "ZZ"
		}
		if _, ok := buckets[code]; !ok {
			order = append(order, code)
		}
		buckets[code] = append(buckets[code], r)
	}
	groups := make([]ShipmentGroup, 0, len(order))
	for _, code := range order {
		groups = append(groups, ShipmentGroup{
			DestinationCountry: code,
			Records:            buckets[code],
		})
	}
	return groups
}

// DeriveMarketMetadata computes all deterministic metadata for a group.
func DeriveMarketMetadata(group ShipmentGroup) MarketMetadata {
	m := MarketMetadata{
		DestinationCountry: group.DestinationCountry,
		ShipmentCount:      len(group.Records),
	}

	// Frequency counts
	hsFreq := make(map[string]int)
	originFreq := make(map[string]int)
	productFreq := make(map[string]int)
	importerKeys := make(map[string]struct{})
	var arrivalDates []time.Time

	for _, r := range group.Records {
		if r.HSCode != "" {
			hsFreq[normalizeHSCode(r.HSCode)]++
		}
		if code := countryToISO2(r.ShipperCountry); code != "" {
			originFreq[code]++
		}
		if p := strings.TrimSpace(r.ProductDescription); p != "" {
			productFreq[strings.ToUpper(p)]++
		}
		key := strings.ToUpper(strings.TrimSpace(r.ConsigneeStdName))
		if key == "" {
			key = strings.ToUpper(strings.TrimSpace(r.ConsigneeName))
		}
		if key != "" {
			importerKeys[key] = struct{}{}
		}
		if d, ok := parseArrivalDate(r.ArrivalDate); ok {
			arrivalDates = append(arrivalDates, d)
		}
	}

	m.ImporterCount = len(importerKeys)
	m.AllHSCodes = sortByFreqDesc(hsFreq)
	m.AllOriginCountries = sortByFreqDesc(originFreq)

	if len(m.AllHSCodes) > 0 {
		m.DominantHSCode = m.AllHSCodes[0]
	}
	if len(m.AllOriginCountries) > 0 {
		m.OriginCountry = m.AllOriginCountries[0]
		totalWithOrigin := 0
		for _, c := range originFreq {
			totalWithOrigin += c
		}
		if totalWithOrigin > 0 {
			m.OriginShare = float64(originFreq[m.OriginCountry]) / float64(totalWithOrigin)
		}
	}

	// Top product descriptions (preserve original casing by looking up from first occurrence)
	topKeys := sortByFreqDesc(productFreq)
	if len(topKeys) > 20 {
		topKeys = topKeys[:20]
	}
	caseMap := make(map[string]string)
	for _, r := range group.Records {
		up := strings.ToUpper(strings.TrimSpace(r.ProductDescription))
		if up == "" {
			continue
		}
		if _, ok := caseMap[up]; !ok {
			caseMap[up] = strings.TrimSpace(r.ProductDescription)
		}
	}
	m.TopProductDescs = make([]string, 0, len(topKeys))
	for _, k := range topKeys {
		if v, ok := caseMap[k]; ok {
			m.TopProductDescs = append(m.TopProductDescs, v)
		}
	}

	// Date range: 5th and 95th percentile
	if len(arrivalDates) > 0 {
		sort.Slice(arrivalDates, func(i, j int) bool { return arrivalDates[i].Before(arrivalDates[j]) })
		m.ShipmentFromDate = percentileDate(arrivalDates, 0.05)
		m.ShipmentToDate = percentileDate(arrivalDates, 0.95)
	}

	return m
}

// percentileDate returns the date at the given percentile (0.0-1.0) from a sorted slice.
func percentileDate(sorted []time.Time, p float64) time.Time {
	if len(sorted) == 0 {
		return time.Time{}
	}
	idx := int(float64(len(sorted)-1) * p)
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// sortByFreqDesc returns map keys ordered by frequency descending, then alphabetically.
func sortByFreqDesc(freq map[string]int) []string {
	keys := make([]string, 0, len(freq))
	for k := range freq {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if freq[keys[i]] != freq[keys[j]] {
			return freq[keys[i]] > freq[keys[j]]
		}
		return keys[i] < keys[j]
	})
	return keys
}

// normalizeHSCode trims to 6 digits (the subheading level), stripping dots/whitespace.
func normalizeHSCode(raw string) string {
	cleaned := strings.Builder{}
	for _, ch := range raw {
		if ch >= '0' && ch <= '9' {
			cleaned.WriteRune(ch)
		}
	}
	s := cleaned.String()
	if len(s) > 6 {
		s = s[:6]
	}
	// Format as "XXXX.XX" for display consistency
	if len(s) >= 5 {
		return s[:4] + "." + s[4:]
	}
	return s
}

// parseArrivalDate tries several common formats.
func parseArrivalDate(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	// Strip trailing time component if present
	if i := strings.Index(raw, " "); i > 0 && len(raw) > 10 {
		raw = raw[:i]
	}
	formats := []string{
		"2006-01-02",
		"2006/01/02",
		"01/02/2006",
		"02/01/2006",
		"2006.01.02",
		"02.01.2006",
		"2006-1-2",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, raw); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// countryToISO2 maps the most common customs-data country strings to ISO-2 codes.
// Returns "" if the input cannot be recognized.
func countryToISO2(raw string) string {
	s := strings.ToUpper(strings.TrimSpace(raw))
	if s == "" {
		return ""
	}
	// Already an ISO-2 code
	if len(s) == 2 && isAlpha(s) {
		return s
	}
	// Strip parenthetical qualifiers like "UNITED STATES(EN)"
	if i := strings.Index(s, "("); i > 0 {
		s = strings.TrimSpace(s[:i])
	}
	m := map[string]string{
		"TURKEY": "TR", "TURKIYE": "TR", "TÜRKIYE": "TR",
		"UNITED STATES": "US", "UNITED STATES OF AMERICA": "US", "USA": "US",
		"GERMANY": "DE", "DEUTSCHLAND": "DE",
		"UNITED KINGDOM": "GB", "ENGLAND": "GB", "GREAT BRITAIN": "GB", "UK": "GB",
		"FRANCE": "FR", "ITALY": "IT", "ITALIA": "IT",
		"SPAIN": "ES", "ESPAÑA": "ES", "ESPANA": "ES",
		"NETHERLANDS": "NL", "HOLLAND": "NL",
		"BELGIUM": "BE", "LUXEMBOURG": "LU",
		"CHINA": "CN", "HONG KONG": "HK", "TAIWAN": "TW",
		"JAPAN": "JP", "KOREA": "KR", "SOUTH KOREA": "KR", "REPUBLIC OF KOREA": "KR",
		"SAUDI ARABIA": "SA", "UNITED ARAB EMIRATES": "AE", "UAE": "AE",
		"BRAZIL": "BR", "BRASIL": "BR",
		"INDIA": "IN", "RUSSIA": "RU", "RUSSIAN FEDERATION": "RU",
		"CANADA": "CA", "MEXICO": "MX", "AUSTRALIA": "AU",
		"POLAND": "PL", "GREECE": "GR", "PORTUGAL": "PT",
		"SWEDEN": "SE", "NORWAY": "NO", "DENMARK": "DK", "FINLAND": "FI",
		"IRELAND": "IE", "SWITZERLAND": "CH", "AUSTRIA": "AT",
		"ISRAEL": "IL", "EGYPT": "EG", "MOROCCO": "MA", "SOUTH AFRICA": "ZA",
		"ARGENTINA": "AR", "CHILE": "CL", "COLOMBIA": "CO",
		"THAILAND": "TH", "VIETNAM": "VN", "INDONESIA": "ID",
		"MALAYSIA": "MY", "PHILIPPINES": "PH", "SINGAPORE": "SG",
	}
	return m[s]
}

func isAlpha(s string) bool {
	for _, ch := range s {
		if (ch < 'A' || ch > 'Z') && (ch < 'a' || ch > 'z') {
			return false
		}
	}
	return true
}
