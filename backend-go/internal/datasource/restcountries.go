package datasource

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/batuhan/marketintel/internal/domain"
)

// RESTCountries fetches country info from REST Countries API (v3.1).
type RESTCountries struct {
	baseURL string
	client  *http.Client
}

func NewRESTCountries(baseURL string) *RESTCountries {
	if baseURL == "" {
		baseURL = "https://restcountries.com/v3.1"
	}
	return &RESTCountries{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: 15 * time.Second},
	}
}

// FetchCountryInfo retrieves basic country data by ISO alpha-2 code.
func (rc *RESTCountries) FetchCountryInfo(ctx context.Context, countryCode string) (*domain.CountryInfo, error) {
	u := fmt.Sprintf("%s/alpha/%s?fields=name,population,currencies,languages,region,cca2", rc.baseURL, countryCode)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}

	resp, err := rc.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("restcountries: request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		switch resp.StatusCode {
		case http.StatusNotFound:
			return nil, fmt.Errorf("restcountries: country code %q not recognized", countryCode)
		case http.StatusTooManyRequests:
			return nil, fmt.Errorf("restcountries: rate limited (HTTP 429)")
		case http.StatusServiceUnavailable, http.StatusBadGateway:
			return nil, fmt.Errorf("restcountries: service temporarily unavailable (HTTP %d)", resp.StatusCode)
		default:
			return nil, fmt.Errorf("restcountries: HTTP %d for %s", resp.StatusCode, countryCode)
		}
	}

	var raw restCountryResp
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("restcountries: decode: %w", err)
	}

	return &domain.CountryInfo{
		Code:       countryCode,
		Name:       raw.Name.Common,
		Population: raw.Population,
		Region:     raw.Region,
		Currencies: raw.currenciesString(),
		Languages:  raw.languagesString(),
	}, nil
}

type restCountryResp struct {
	Name struct {
		Common string `json:"common"`
	} `json:"name"`
	CCA2       string                       `json:"cca2"`
	Population int64                        `json:"population"`
	Region     string                       `json:"region"`
	Currencies map[string]restCurrencyInfo  `json:"currencies"`
	Languages  map[string]string            `json:"languages"`
}

type restCurrencyInfo struct {
	Name   string `json:"name"`
	Symbol string `json:"symbol"`
}

func (r *restCountryResp) currenciesString() string {
	parts := make([]string, 0, len(r.Currencies))
	for code, info := range r.Currencies {
		parts = append(parts, fmt.Sprintf("%s (%s)", info.Name, code))
	}
	return strings.Join(parts, ", ")
}

func (r *restCountryResp) languagesString() string {
	parts := make([]string, 0, len(r.Languages))
	for _, name := range r.Languages {
		parts = append(parts, name)
	}
	return strings.Join(parts, ", ")
}
