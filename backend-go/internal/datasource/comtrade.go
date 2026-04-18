package datasource

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/batuhan/marketintel/internal/domain"
)

// normalizeHS strips dots from HS codes (e.g. "6802.91" → "680291")
// because Comtrade API expects raw numeric codes.
func normalizeHS(code string) string {
	return strings.ReplaceAll(code, ".", "")
}

const comtradeBaseURL = "https://comtradeapi.un.org/public/v1/preview/C/A/HS"

// Comtrade fetches international trade flow data from UN Comtrade API.
type Comtrade struct {
	apiKey string
	client *http.Client
}

func NewComtrade(apiKey string) *Comtrade {
	return &Comtrade{
		apiKey: apiKey,
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

// FetchTradeData gets import/export flows for an HS code in a specific country.
// Returns aggregated TradeData with top partners.
func (c *Comtrade) FetchTradeData(ctx context.Context, hsCode, countryCode string) (*domain.TradeData, error) {
	hsCode = normalizeHS(hsCode)
	m49 := isoToM49(countryCode)
	if m49 == "" {
		return nil, fmt.Errorf("comtrade: unknown country code %q", countryCode)
	}

	// Fetch imports and exports in sequence (Comtrade rate limits are tight)
	imports, err := c.fetchFlows(ctx, hsCode, m49, "M")
	// Fallback to 4-digit parent if 6-digit code fails
	if (err != nil || len(imports) == 0) && len(hsCode) > 4 {
		parent := hsCode[:4]
		slog.Info("comtrade: falling back to 4-digit heading", "from", hsCode, "to", parent)
		hsCode = parent
		imports, err = c.fetchFlows(ctx, hsCode, m49, "M")
	}
	if err != nil {
		return nil, fmt.Errorf("comtrade: fetch imports: %w", err)
	}

	exports, err := c.fetchFlows(ctx, hsCode, m49, "X")
	if err != nil {
		return nil, fmt.Errorf("comtrade: fetch exports: %w", err)
	}

	td := &domain.TradeData{
		HSCode:      hsCode,
		CountryCode: countryCode,
	}

	// Aggregate import data
	for _, r := range imports {
		td.ImportValueUSD += r.TradeValueUSD
	}
	td.TopExporters = topPartners(imports, 10)

	// Aggregate export data
	for _, r := range exports {
		td.ExportValueUSD += r.TradeValueUSD
	}
	td.TopImporters = topPartners(exports, 10)

	if len(imports) > 0 || len(exports) > 0 {
		td.YearRange = "recent"
	}

	return td, nil
}

type comtradeRecord struct {
	ReporterCode int64   `json:"reporterCode"`
	ReporterDesc string  `json:"reporterDesc"`
	PartnerCode  int64   `json:"partnerCode"`
	PartnerDesc  string  `json:"partnerDesc"`
	Partner2Code int64   `json:"partner2Code"`
	Partner2Desc *string `json:"partner2Desc"`
	CustomsCode  string  `json:"customsCode"`
	MotCode      int64   `json:"motCode"`
	CmdCode      string  `json:"cmdCode"`
	FlowCode     string  `json:"flowCode"`
	Period       string  `json:"period"`
	PrimaryValue float64 `json:"primaryValue"`
}

func (c *Comtrade) fetchFlows(ctx context.Context, hsCode, reporterM49, flow string) ([]tradeRow, error) {
	params := url.Values{
		"cmdCode":      {hsCode},
		"flowCode":     {flow},
		"reporterCode": {reporterM49},
		"partnerCode":  {"0"},
		"period":       {"2023"},
	}
	if c.apiKey != "" {
		params.Set("subscription-key", c.apiKey)
	}

	reqURL := comtradeBaseURL + "?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("comtrade: request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		switch resp.StatusCode {
		case http.StatusUnauthorized:
			return nil, fmt.Errorf("comtrade: API key invalid or expired (HTTP 401)")
		case http.StatusForbidden:
			return nil, fmt.Errorf("comtrade: access denied — subscription may not cover this endpoint (HTTP 403)")
		case http.StatusTooManyRequests:
			return nil, fmt.Errorf("comtrade: rate limited (HTTP 429) — Comtrade allows limited requests per minute")
		case http.StatusServiceUnavailable, http.StatusBadGateway:
			return nil, fmt.Errorf("comtrade: service temporarily unavailable (HTTP %d) — try again later", resp.StatusCode)
		default:
			return nil, fmt.Errorf("comtrade: HTTP %d for HS %s flow=%s: %s", resp.StatusCode, hsCode, flow, string(bodyBytes))
		}
	}

	var data struct {
		Data []comtradeRecord `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("comtrade: decode: %w", err)
	}

	// Deduplicate & aggregate: the preview API now returns rows broken out by
	// customsCode (C00=general, C03=special trade) and motCode (mode of transport).
	// partnerCode is always 0; the real partner is in partner2Code.
	// Strategy: pick one customsCode per partner (prefer C00, fall back to C03),
	// then use motCode=0 row as the total if present, otherwise sum non-zero motCodes.

	// Step 1: group records by (partner2Code, customsCode)
	type groupKey struct {
		partner2   int64
		customsCode string
	}
	groups := make(map[groupKey][]comtradeRecord)
	for _, r := range data.Data {
		if r.Partner2Code == 0 {
			continue // skip "World" aggregate rows
		}
		k := groupKey{partner2: r.Partner2Code, customsCode: r.CustomsCode}
		groups[k] = append(groups[k], r)
	}

	// Step 2: for each partner, compute value per customsCode, then pick the best
	partnerValues := make(map[int64]int64)
	partnerCustoms := make(map[int64]string) // track which customsCode was chosen

	for k, recs := range groups {
		// If motCode=0 (aggregate) exists, use it; otherwise sum all motCodes
		var val float64
		hasMotZero := false
		for _, r := range recs {
			if r.MotCode == 0 {
				val += r.PrimaryValue
				hasMotZero = true
			}
		}
		if !hasMotZero {
			for _, r := range recs {
				val += r.PrimaryValue
			}
		}

		intVal := int64(val)
		prev, exists := partnerValues[k.partner2]
		if !exists {
			partnerValues[k.partner2] = intVal
			partnerCustoms[k.partner2] = k.customsCode
		} else {
			// Prefer C00 over C03; if same customsCode priority, pick higher value
			prevCC := partnerCustoms[k.partner2]
			preferNew := false
			if prevCC == k.customsCode {
				preferNew = intVal > prev
			} else if k.customsCode == "C00" {
				preferNew = true
			}
			if preferNew {
				partnerValues[k.partner2] = intVal
				partnerCustoms[k.partner2] = k.customsCode
			}
		}
	}

	rows := make([]tradeRow, 0, len(partnerValues))
	for code, val := range partnerValues {
		rows = append(rows, tradeRow{
			PartnerCode:   fmt.Sprintf("%d", code),
			PartnerName:   "",
			TradeValueUSD: val,
		})
	}
	return rows, nil
}

type tradeRow struct {
	PartnerCode   string
	PartnerName   string
	TradeValueUSD int64
}

func topPartners(rows []tradeRow, n int) []domain.TradePartner {
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].TradeValueUSD > rows[j].TradeValueUSD
	})

	var total int64
	for _, r := range rows {
		total += r.TradeValueUSD
	}

	if len(rows) > n {
		rows = rows[:n]
	}

	partners := make([]domain.TradePartner, len(rows))
	for i, r := range rows {
		var share float64
		if total > 0 {
			share = float64(r.TradeValueUSD) / float64(total) * 100
		}
		partners[i] = domain.TradePartner{
			CountryCode: r.PartnerCode,
			CountryName: r.PartnerName,
			ValueUSD:    r.TradeValueUSD,
			SharePct:    share,
		}
	}
	return partners
}

// FetchTopMarkets returns the top N countries that import the given HS code from Turkey,
// ranked by trade value. Used for Stage 2 market ranking.
func (c *Comtrade) FetchTopMarkets(ctx context.Context, hsCode string, limit int) ([]domain.TradePartner, error) {
	hsCode = normalizeHS(hsCode)
	// Query Turkey's exports (reporter=792/TR, flow=X) to find destination countries
	rows, err := c.fetchFlows(ctx, hsCode, "792", "X")
	// Fallback to 4-digit parent if 6-digit code returns no data or errors
	if (err != nil || len(rows) == 0) && len(hsCode) > 4 {
		parent := hsCode[:4]
		slog.Info("comtrade: falling back to 4-digit heading", "from", hsCode, "to", parent)
		rows, err = c.fetchFlows(ctx, parent, "792", "X")
	}
	if err != nil {
		return nil, fmt.Errorf("comtrade: fetch top markets: %w", err)
	}
	partners := topPartners(rows, limit)

	// Convert M49 codes to ISO alpha-2 for downstream use
	result := make([]domain.TradePartner, 0, len(partners))
	for _, p := range partners {
		if iso := m49ToISO(p.CountryCode); iso != "" {
			p.CountryCode = iso
			result = append(result, p)
		}
	}
	return result, nil
}

// m49ToISO converts UN M49 numeric code to ISO 3166-1 alpha-2.
func m49ToISO(m49 string) string {
	m := map[string]string{
		"4": "AF", "8": "AL", "12": "DZ", "20": "AD", "24": "AO",
		"31": "AZ", "32": "AR", "36": "AU", "40": "AT", "44": "BS",
		"48": "BH", "50": "BD", "51": "AM", "56": "BE", "60": "BM",
		"64": "BT", "68": "BO", "70": "BA", "72": "BW", "76": "BR",
		"84": "BZ", "90": "SB", "96": "BN", "100": "BG", "104": "MM",
		"108": "BI", "112": "BY", "116": "KH", "120": "CM", "124": "CA",
		"132": "CV", "144": "LK", "148": "TD", "152": "CL", "156": "CN",
		"158": "TW", "170": "CO", "174": "KM", "178": "CG", "180": "CD",
		"188": "CR", "191": "HR", "192": "CU", "196": "CY", "203": "CZ",
		"204": "BJ", "208": "DK", "214": "DO", "218": "EC", "222": "SV",
		"226": "GQ", "231": "ET", "233": "EE", "242": "FJ", "246": "FI",
		"250": "FR", "251": "FR", "266": "GA", "268": "GE", "270": "GM", "275": "PS",
		"276": "DE", "288": "GH", "300": "GR", "320": "GT", "324": "GN",
		"328": "GY", "332": "HT", "340": "HN", "344": "HK", "348": "HU",
		"352": "IS", "356": "IN", "360": "ID", "364": "IR", "368": "IQ",
		"372": "IE", "376": "IL", "380": "IT", "384": "CI", "388": "JM",
		"392": "JP", "398": "KZ", "400": "JO", "404": "KE", "408": "KP",
		"410": "KR", "414": "KW", "417": "KG", "418": "LA", "422": "LB",
		"426": "LS", "428": "LV", "430": "LR", "434": "LY", "440": "LT",
		"442": "LU", "450": "MG", "454": "MW", "458": "MY", "462": "MV",
		"466": "ML", "470": "MT", "478": "MR", "480": "MU", "484": "MX",
		"490": "OT", "496": "MN", "498": "MD", "504": "MA", "508": "MZ",
		"512": "OM", "516": "NA", "524": "NP", "528": "NL", "540": "NC",
		"548": "VU", "554": "NZ", "558": "NI", "562": "NE", "566": "NG",
		"578": "NO", "586": "PK", "591": "PA", "598": "PG", "600": "PY",
		"604": "PE", "608": "PH", "616": "PL", "620": "PT", "624": "GW",
		"634": "QA", "642": "RO", "643": "RU", "646": "RW", "682": "SA",
		"686": "SN", "688": "RS", "694": "SL", "699": "IN", "700": "SG",
		"702": "SG", "703": "SK", "704": "VN", "705": "SI", "706": "SO",
		"710": "ZA", "716": "ZW", "724": "ES", "729": "SD", "740": "SR",
		"748": "SZ", "752": "SE", "756": "CH", "760": "SY", "762": "TJ",
		"764": "TH", "768": "TG", "776": "TO", "780": "TT", "784": "AE",
		"788": "TN", "792": "TR", "795": "TM", "800": "UG", "804": "UA",
		"807": "MK", "818": "EG", "826": "GB", "834": "TZ", "840": "US",
		"842": "US", "854": "BF", "858": "UY", "860": "UZ", "862": "VE",
		"887": "YE", "894": "ZM",
	}
	return m[m49]
}

// isoToM49 converts ISO 3166-1 alpha-2 to UN M49 numeric code.
func isoToM49(iso string) string {
	m := map[string]string{
		"TR": "792", "DE": "276", "US": "840", "GB": "826",
		"FR": "250", "IT": "380", "ES": "724", "NL": "528",
		"SA": "682", "AE": "784", "EG": "818", "RU": "643",
		"CN": "156", "JP": "392", "IN": "356", "BR": "076",
		"IQ": "368", "PL": "616", "SE": "752", "AT": "040",
		"BE": "056", "CH": "756", "CZ": "203", "DK": "208",
		"FI": "246", "GR": "300", "HU": "348", "IE": "372",
		"NO": "578", "PT": "620", "RO": "642", "SK": "703",
		"AU": "036", "CA": "124", "MX": "484", "KR": "410",
		"IL": "376", "ZA": "710", "NG": "566", "KE": "404",
		"MA": "504", "TN": "788", "PK": "586", "BD": "050",
		"VN": "704", "TH": "764", "ID": "360", "MY": "458",
		"PH": "608", "SG": "702", "DZ": "012", "AR": "032",
		"CL": "152", "CO": "170", "PE": "604", "EC": "218",
		"JO": "400", "LB": "422", "KW": "414", "QA": "634",
		"BH": "048", "OM": "512", "LY": "434", "SD": "729",
		"ET": "231", "GH": "288", "TZ": "834", "UG": "800",
		"UA": "804", "RS": "688", "HR": "191", "BG": "100",
		"GE": "268", "KZ": "398", "UZ": "860", "HK": "344",
		"TW": "158", "MM": "104", "KH": "116", "LK": "144",
		"NP": "524", "BA": "070", "SI": "705",
	}
	return m[iso]
}
