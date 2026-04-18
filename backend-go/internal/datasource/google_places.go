package datasource

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/batuhan/marketintel/internal/domain"
)

const (
	placesTextSearchURL = "https://places.googleapis.com/v1/places:searchText"
	geocodeURL          = "https://maps.googleapis.com/maps/api/geocode/json"
	placesFieldMask = "places.id,places.displayName,places.formattedAddress," +
		"places.location,places.types,places.websiteUri," +
		"places.nationalPhoneNumber,places.businessStatus," +
		"places.rating,places.userRatingCount," +
		"places.primaryType,places.primaryTypeDisplayName," +
		"places.currentOpeningHours,places.editorialSummary"
)

// GooglePlaces discovers businesses via Google Places API (New).
// Implements domain.BusinessFinder.
type GooglePlaces struct {
	apiKey string
	client *http.Client
}

func NewGooglePlaces(apiKey string) *GooglePlaces {
	return &GooglePlaces{
		apiKey: apiKey,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// SearchBusinesses runs multiple buyer-focused queries against Google Places,
// deduplicates by google_place_id, and returns up to maxResults businesses.
func (gp *GooglePlaces) SearchBusinesses(
	ctx context.Context,
	queries []string,
	lat, lng float64,
	radiusKM int,
	maxResults int,
) ([]domain.Business, error) {
	if gp.apiKey == "" {
		return nil, fmt.Errorf("google_places: API key not configured (set GOOGLE_PLACES_API_KEY)")
	}
	if lat == 0 && lng == 0 {
		return nil, fmt.Errorf("google_places: lat/lng required (use Geocode first)")
	}

	seen := make(map[string]struct{})
	var results []domain.Business

	for _, q := range queries {
		if len(results) >= maxResults {
			break
		}

		places, err := gp.textSearch(ctx, q, lat, lng, radiusKM)
		if err != nil {
			slog.Warn("google_places: search failed", "query", q, "error", err)
			continue
		}

		for _, b := range places {
			if b.GooglePlaceID == nil {
				continue
			}
			if _, dup := seen[*b.GooglePlaceID]; dup {
				continue
			}
			seen[*b.GooglePlaceID] = struct{}{}
			results = append(results, b)
			if len(results) >= maxResults {
				break
			}
		}
	}

	return results, nil
}

// Geocode resolves a city + country to lat/lng using Google Geocoding API.
func (gp *GooglePlaces) Geocode(ctx context.Context, city, countryCode string) (float64, float64, error) {
	address := city
	if countryCode != "" {
		address = city + ", " + countryCode
	}

	u := geocodeURL + "?" + url.Values{
		"address": {address},
		"key":     {gp.apiKey},
	}.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0, 0, err
	}

	resp, err := gp.client.Do(req)
	if err != nil {
		return 0, 0, fmt.Errorf("google_places: geocode request: %w", err)
	}
	defer resp.Body.Close()

	var data struct {
		Results []struct {
			Geometry struct {
				Location struct {
					Lat float64 `json:"lat"`
					Lng float64 `json:"lng"`
				} `json:"location"`
			} `json:"geometry"`
		} `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return 0, 0, fmt.Errorf("google_places: geocode decode: %w", err)
	}

	if len(data.Results) == 0 {
		return 0, 0, fmt.Errorf("google_places: geocode no results for %q", address)
	}

	loc := data.Results[0].Geometry.Location
	return loc.Lat, loc.Lng, nil
}

// SearchByName does a targeted text search for a specific company.
// Used for verifying Tendata businesses against Google Places.
func (gp *GooglePlaces) SearchByName(ctx context.Context, companyName, city, countryCode string) ([]domain.Business, error) {
	if gp.apiKey == "" {
		return nil, fmt.Errorf("google_places: API key not configured (set GOOGLE_PLACES_API_KEY)")
	}

	var query string
	if city != "" {
		query = fmt.Sprintf("%q %s", companyName, city)
	} else {
		query = fmt.Sprintf("%q %s", companyName, countryCode)
	}

	body := map[string]any{
		"textQuery":      query,
		"maxResultCount": 3,
	}

	// Use city geocode as a location bias if available.
	if city != "" {
		lat, lng, err := gp.Geocode(ctx, city, countryCode)
		if err == nil && (lat != 0 || lng != 0) {
			body["locationBias"] = map[string]any{
				"circle": map[string]any{
					"center": map[string]float64{
						"latitude":  lat,
						"longitude": lng,
					},
					"radius": 50000.0, // 50 km — general city area
				},
			}
		}
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, placesTextSearchURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Goog-Api-Key", gp.apiKey)
	req.Header.Set("X-Goog-FieldMask", placesFieldMask)

	resp, err := gp.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("google_places: search by name: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("google_places: search by name HTTP %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var data struct {
		Places []placesResult `json:"places"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("google_places: search by name decode: %w", err)
	}

	businesses := make([]domain.Business, 0, len(data.Places))
	for _, p := range data.Places {
		businesses = append(businesses, p.toBusiness())
	}
	return businesses, nil
}

// textSearch calls Google Places Text Search (New) for a single query.
func (gp *GooglePlaces) textSearch(ctx context.Context, query string, lat, lng float64, radiusKM int) ([]domain.Business, error) {
	body := map[string]any{
		"textQuery":      query,
		"maxResultCount": 20, // API max per request
	}

	if lat != 0 || lng != 0 {
		body["locationBias"] = map[string]any{
			"circle": map[string]any{
				"center": map[string]float64{
					"latitude":  lat,
					"longitude": lng,
				},
				"radius": float64(radiusKM * 1000),
			},
		}
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, placesTextSearchURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Goog-Api-Key", gp.apiKey)
	req.Header.Set("X-Goog-FieldMask", placesFieldMask)

	resp, err := gp.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("google_places: text search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		switch resp.StatusCode {
		case http.StatusBadRequest:
			return nil, fmt.Errorf("google_places: bad request — check API key and query: %s", string(bodyBytes))
		case http.StatusForbidden:
			return nil, fmt.Errorf("google_places: API key invalid or Places API not enabled in Google Cloud Console (HTTP 403)")
		case http.StatusTooManyRequests:
			return nil, fmt.Errorf("google_places: rate limited (HTTP 429) — you may have exceeded your quota")
		default:
			return nil, fmt.Errorf("google_places: HTTP %d: %s", resp.StatusCode, string(bodyBytes))
		}
	}

	var data struct {
		Places []placesResult `json:"places"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("google_places: decode: %w", err)
	}

	businesses := make([]domain.Business, 0, len(data.Places))
	for _, p := range data.Places {
		businesses = append(businesses, p.toBusiness())
	}
	return businesses, nil
}

// placesResult maps the Google Places API response.
type placesResult struct {
	ID                     string              `json:"id"`
	DisplayName            localizedText       `json:"displayName"`
	FormattedAddress       string              `json:"formattedAddress"`
	Location               latLng              `json:"location"`
	Types                  []string            `json:"types"`
	WebsiteURI             string              `json:"websiteUri"`
	NationalPhoneNumber    string              `json:"nationalPhoneNumber"`
	BusinessStatus         string              `json:"businessStatus"`
	Rating                 float64             `json:"rating"`
	UserRatingCount        int                 `json:"userRatingCount"`
	PrimaryType            string              `json:"primaryType"`
	PrimaryTypeDisplayName localizedText       `json:"primaryTypeDisplayName"`
	EditorialSummary       *localizedText      `json:"editorialSummary"`
	CurrentOpeningHours    *placesOpeningHours `json:"currentOpeningHours"`
}

type placesOpeningHours struct {
	WeekdayDescriptions []string `json:"weekdayDescriptions"`
}

type localizedText struct {
	Text string `json:"text"`
}

type latLng struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

func (p *placesResult) toBusiness() domain.Business {
	placeID := p.ID
	dataSource := "google_places"

	var website *string
	if p.WebsiteURI != "" {
		website = &p.WebsiteURI
	}

	var phone *string
	if p.NationalPhoneNumber != "" {
		phone = &p.NationalPhoneNumber
	}

	var description *string
	if p.EditorialSummary != nil && p.EditorialSummary.Text != "" {
		description = &p.EditorialSummary.Text
	}

	var primaryType *string
	if p.PrimaryType != "" {
		pt := p.PrimaryType
		primaryType = &pt
	}

	lat := decimal.NewFromFloat(p.Location.Latitude)
	lng := decimal.NewFromFloat(p.Location.Longitude)



	googleTypesJSON, _ := json.Marshal(p.Types)

	var openingHoursJSON json.RawMessage
	if p.CurrentOpeningHours != nil && len(p.CurrentOpeningHours.WeekdayDescriptions) > 0 {
		openingHoursJSON, _ = json.Marshal(p.CurrentOpeningHours.WeekdayDescriptions)
	}

	var rating *float64
	if p.Rating > 0 {
		r := p.Rating
		rating = &r
	}

	var ratingCount *int
	if p.UserRatingCount > 0 {
		rc := p.UserRatingCount
		ratingCount = &rc
	}

	// Extract city from formatted address (rough heuristic — second-to-last comma segment)
	var city *string
	parts := strings.Split(p.FormattedAddress, ",")
	if len(parts) >= 2 {
		c := strings.TrimSpace(parts[len(parts)-2])
		city = &c
	}

	return domain.Business{
		ID:               uuid.New(),
		Name:             p.DisplayName.Text,
		GooglePlaceID:    &placeID,
		Address:          &p.FormattedAddress,
		Latitude:         &lat,
		Longitude:        &lng,
		Website:          website,
		Phone:            phone,
		Description:      description,
		BusinessType:     primaryType,
		City:             city,
		Rating:           rating,
		RatingCount:      ratingCount,
		GoogleTypes:      googleTypesJSON,
		OpeningHours:     openingHoursJSON,
		DataSource:       &dataSource,
		EnrichmentStatus: "pending",
		SocialLinks:      nil,
	}
}
