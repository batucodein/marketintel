package datasource

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/batuhan/marketintel/internal/domain"
)

// Tendata fetches confirmed importer data from Tendata Open API.
// API docs: https://open-api.tendata.cn
// Implements domain.ImporterFetcher.
type Tendata struct {
	apiKey    string
	apiSecret string
	baseURL   string
	client    *http.Client

	mu        sync.Mutex
	token     string
	tokenExp  time.Time
	authFailed bool // sticky flag: once auth fails, don't keep retrying
}

func NewTendata(apiKey, apiSecret, baseURL string) *Tendata {
	if baseURL == "" {
		baseURL = "https://open-api.tendata.cn"
	}
	return &Tendata{
		apiKey:    apiKey,
		apiSecret: apiSecret,
		baseURL:   baseURL,
		client:    &http.Client{Timeout: 30 * time.Second},
	}
}

// FetchImporterCounts returns importer counts for multiple countries (lightweight).
// Used in Stage 2 to show counts on the market ranking screen.
func (t *Tendata) FetchImporterCounts(ctx context.Context, hsCode string, countryCodes []string) ([]domain.ImporterCount, error) {
	hsCode = normalizeHS(hsCode)
	if t.apiKey == "" {
		return nil, fmt.Errorf("tendata: API key not configured")
	}

	t.mu.Lock()
	failed := t.authFailed
	t.mu.Unlock()
	if failed {
		return nil, fmt.Errorf("tendata: API credentials are invalid")
	}

	token, err := t.ensureToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("tendata: auth: %w", err)
	}

	results := make([]domain.ImporterCount, 0, len(countryCodes))
	for _, cc := range countryCodes {
		body, _ := json.Marshal(map[string]any{
			"hs_code":      hsCode,
			"country_code": cc,
			"page":         1,
			"page_size":    1, // just need the total count
		})

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.baseURL+"/api/v1/buyers/search", bytes.NewReader(body))
		if err != nil {
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := t.client.Do(req)
		if err != nil {
			continue
		}

		var data struct {
			Code  int `json:"code"`
			Total int `json:"total"`
		}
		json.NewDecoder(resp.Body).Decode(&data)
		resp.Body.Close()

		if data.Code == 0 {
			results = append(results, domain.ImporterCount{CountryCode: cc, Count: data.Total})
		}
	}

	return results, nil
}

// FetchImporters searches for confirmed importers by HS code and country.
func (t *Tendata) FetchImporters(ctx context.Context, hsCode, countryCode string, city *string) ([]domain.ConfirmedImporter, error) {
	hsCode = normalizeHS(hsCode)
	if t.apiKey == "" {
		return nil, fmt.Errorf("tendata: API key not configured")
	}

	// If auth already failed permanently, don't waste time retrying
	t.mu.Lock()
	failed := t.authFailed
	t.mu.Unlock()
	if failed {
		return nil, fmt.Errorf("tendata: API credentials are invalid (auth previously failed)")
	}

	token, err := t.ensureToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("tendata: auth: %w", err)
	}

	body := map[string]any{
		"hs_code":      hsCode,
		"country_code": countryCode,
		"page":         1,
		"page_size":    50,
	}
	if city != nil && *city != "" {
		body["city"] = *city
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.baseURL+"/api/v1/buyers/search", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tendata: request: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		// continue below
	case http.StatusUnauthorized:
		// Token may have expired mid-request. Retry ONCE with a fresh token.
		t.clearToken()
		token2, err := t.ensureToken(ctx)
		if err != nil {
			return nil, fmt.Errorf("tendata: re-auth failed: %w", err)
		}
		return t.fetchWithToken(ctx, payload, token2)
	case http.StatusForbidden:
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.markAuthFailed()
		return nil, fmt.Errorf("tendata: access forbidden — your API subscription may not include API access (HTTP 403: %s)", string(bodyBytes))
	case http.StatusTooManyRequests:
		return nil, fmt.Errorf("tendata: rate limited (HTTP 429) — retry later")
	case http.StatusPaymentRequired:
		t.markAuthFailed()
		return nil, fmt.Errorf("tendata: credits exhausted (HTTP 402) — top up your Tendata account")
	default:
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("tendata: unexpected status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return t.parseImporters(resp.Body)
}

// fetchWithToken is the single-retry path after a 401.
func (t *Tendata) fetchWithToken(ctx context.Context, payload []byte, token string) ([]domain.ConfirmedImporter, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.baseURL+"/api/v1/buyers/search", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tendata: retry request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		t.markAuthFailed()
		return nil, fmt.Errorf("tendata: API credentials are invalid (401 after token refresh)")
	}
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("tendata: retry status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return t.parseImporters(resp.Body)
}

func (t *Tendata) parseImporters(body io.Reader) ([]domain.ConfirmedImporter, error) {
	var data struct {
		Code    int               `json:"code"`
		Message string            `json:"message"`
		Data    []tendataImporter `json:"data"`
	}
	if err := json.NewDecoder(body).Decode(&data); err != nil {
		return nil, fmt.Errorf("tendata: decode: %w", err)
	}

	if data.Code != 0 {
		// Map known API error codes
		switch data.Code {
		case 401, 1001:
			t.markAuthFailed()
			return nil, fmt.Errorf("tendata: auth error (code %d): %s", data.Code, data.Message)
		case 402, 1002:
			t.markAuthFailed()
			return nil, fmt.Errorf("tendata: insufficient credits (code %d): %s", data.Code, data.Message)
		case 429, 1003:
			return nil, fmt.Errorf("tendata: rate limited (code %d): %s", data.Code, data.Message)
		default:
			return nil, fmt.Errorf("tendata: API error (code %d): %s", data.Code, data.Message)
		}
	}

	importers := make([]domain.ConfirmedImporter, 0, len(data.Data))
	for _, r := range data.Data {
		importers = append(importers, r.toDomain())
	}

	slog.Info("tendata: fetched importers",
		"count", len(importers),
	)

	return importers, nil
}

// ensureToken returns a valid Bearer token, refreshing if expired.
func (t *Tendata) ensureToken(ctx context.Context) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.authFailed {
		return "", fmt.Errorf("credentials previously rejected")
	}

	if t.token != "" && time.Now().Before(t.tokenExp) {
		return t.token, nil
	}

	body, err := json.Marshal(map[string]string{
		"api_key":    t.apiKey,
		"api_secret": t.apiSecret,
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.baseURL+"/api/v1/auth/token", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.authFailed = true
		return "", fmt.Errorf("invalid API credentials (HTTP %d): %s — verify your Tendata API key/secret", resp.StatusCode, string(bodyBytes))
	}

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("token request status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var result struct {
		Code  int    `json:"code"`
		Token string `json:"token"`
		Exp   int64  `json:"expires_in"` // seconds
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("token decode: %w", err)
	}

	if result.Token == "" {
		t.authFailed = true
		return "", fmt.Errorf("empty token — API key may not have API access (web subscription != API access)")
	}

	t.token = result.Token
	// Refresh 5 minutes before actual expiry
	t.tokenExp = time.Now().Add(time.Duration(result.Exp)*time.Second - 5*time.Minute)

	slog.Info("tendata: token refreshed", "expires_in_sec", result.Exp)
	return t.token, nil
}

func (t *Tendata) clearToken() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.token = ""
	t.tokenExp = time.Time{}
}

func (t *Tendata) markAuthFailed() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.authFailed = true
	t.token = ""
}

type tendataImporter struct {
	CompanyName      string  `json:"company_name"`
	Country          string  `json:"country"`
	City             string  `json:"city"`
	Address          string  `json:"address"`
	TransactionCount int     `json:"transaction_count"`
	TotalValueUSD    float64 `json:"total_value_usd"`
	AvgOrderUSD      float64 `json:"avg_order_usd"`
	LastImportDate   string  `json:"last_import_date"`
	TopSuppliers     string  `json:"top_suppliers"`
	ContactEmail     string  `json:"contact_email"`
	ContactPhone     string  `json:"contact_phone"`
	Website          string  `json:"website"`
}

func (r *tendataImporter) toDomain() domain.ConfirmedImporter {
	return domain.ConfirmedImporter{
		CompanyName:      r.CompanyName,
		Country:          r.Country,
		City:             r.City,
		Address:          r.Address,
		TransactionCount: r.TransactionCount,
		TotalValueUSD:    r.TotalValueUSD,
		AvgOrderUSD:      r.AvgOrderUSD,
		LastImportDate:   r.LastImportDate,
		TopSuppliers:     r.TopSuppliers,
		ContactEmail:     r.ContactEmail,
		ContactPhone:     r.ContactPhone,
		Website:          r.Website,
	}
}
