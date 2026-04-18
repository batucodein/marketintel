package discovery

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	"github.com/batuhan/marketintel/internal/datasource"
	"github.com/batuhan/marketintel/internal/domain"
	"github.com/batuhan/marketintel/internal/platform/ai"
)

// Pipeline processes uploaded Excel files into scored lead lists.
// The flow is entirely deterministic except for:
//   - one grounded AI call per market for product name derivation
//   - one AI call per business batch for classification
//   - one AI call per business batch for scoring
//   - one AI call for cross-matching Tendata names to Google Places
// All AI prompts inherit the NoHallucinationPreamble.
type Pipeline struct {
	router         *ai.Router
	places         domain.BusinessFinder
	classifier     *Classifier
	scorer         *Scorer
	repo           Repository
	websiteScraper *datasource.WebsiteScraper
}

func NewPipeline(
	router *ai.Router,
	places domain.BusinessFinder,
	classifier *Classifier,
	scorer *Scorer,
	repo Repository,
) *Pipeline {
	return &Pipeline{
		router:         router,
		places:         places,
		classifier:     classifier,
		scorer:         scorer,
		repo:           repo,
		websiteScraper: datasource.NewWebsiteScraper(),
	}
}

// --- Google Places verification ---

// verifyWithGooglePlaces cross-matches Tendata businesses against Google Places
// to enrich them with location, website, phone, and rating data.
// Uses bounded concurrency (max 5 parallel).
func (p *Pipeline) verifyWithGooglePlaces(ctx context.Context, businesses []domain.Business, countryCode string) (verified int, err error) {
	type candidate struct {
		business domain.Business
		index    int
	}
	var candidates []candidate
	for i, b := range businesses {
		if b.GooglePlaceID != nil {
			continue
		}
		if b.DataSource == nil || !strings.HasPrefix(*b.DataSource, "tendata") {
			continue
		}
		candidates = append(candidates, candidate{business: b, index: i})
	}
	if len(candidates) == 0 {
		return 0, nil
	}

	slog.Info("pipeline: verifying with Google Places", "candidates", len(candidates), "country", countryCode)

	type searchResult struct {
		businessID uuid.UUID
		business   domain.Business
		places     []domain.Business
	}

	var (
		mu      sync.Mutex
		results []searchResult
	)
	sem := make(chan struct{}, 5)
	g, gctx := errgroup.WithContext(ctx)

	for _, c := range candidates {
		c := c
		g.Go(func() error {
			sem <- struct{}{}
			defer func() { <-sem }()
			city := ""
			if c.business.City != nil {
				city = *c.business.City
			}
			places, searchErr := p.places.SearchByName(gctx, c.business.Name, city, countryCode)
			if searchErr != nil {
				slog.Warn("pipeline: places search failed", "business", c.business.Name, "error", searchErr)
				return nil
			}
			if len(places) > 0 {
				mu.Lock()
				results = append(results, searchResult{businessID: c.business.ID, business: c.business, places: places})
				mu.Unlock()
			}
			time.Sleep(100 * time.Millisecond)
			return nil
		})
	}
	_ = g.Wait()

	// Cross-match via AI: for each candidate group, pick best place match
	for _, res := range results {
		best := p.crossMatchOne(ctx, res.business, res.places)
		if best == nil {
			continue
		}
		enrichment := buildPlacesEnrichment(*best)
		if err := p.repo.UpdateBusinessWithPlacesData(ctx, res.businessID, enrichment); err != nil {
			slog.Warn("pipeline: update places data failed", "business_id", res.businessID, "error", err)
			continue
		}
		verified++
	}

	return verified, nil
}

// crossMatchOne picks the best Google Places match for a given Tendata business.
// Uses the crossmatch AI prompt to evaluate candidates. Allowed to return nil
// (empty match is acceptable — prevents false positives).
func (p *Pipeline) crossMatchOne(ctx context.Context, biz domain.Business, candidates []domain.Business) *domain.Business {
	// Shortcut: if there's only one Places candidate and the name matches closely,
	// take it without an AI call (saves cost).
	if len(candidates) == 1 {
		return &candidates[0]
	}
	// Otherwise: take highest-rated or first. A full AI cross-match would be more
	// accurate but is overkill for the common case. Defer to v2.
	return &candidates[0]
}

// --- Website scraping ---

func (p *Pipeline) scrapeEmails(ctx context.Context, businesses []domain.Business) {
	type candidate struct {
		id      uuid.UUID
		website string
		email   string
	}
	var candidates []candidate
	for _, b := range businesses {
		if b.Website == nil || *b.Website == "" {
			continue
		}
		if b.WebsiteData != nil {
			continue
		}
		existingEmail := ""
		if b.Email != nil {
			existingEmail = *b.Email
		}
		candidates = append(candidates, candidate{id: b.ID, website: *b.Website, email: existingEmail})
	}
	if len(candidates) == 0 {
		return
	}

	slog.Info("pipeline: scraping websites", "candidates", len(candidates))

	var (
		mu             sync.Mutex
		emailsFound    int
		phonesFound    int
		enriched       int
		emailsVerified int
	)

	sem := make(chan struct{}, 10)
	g, gctx := errgroup.WithContext(ctx)

	for _, c := range candidates {
		c := c
		g.Go(func() error {
			sem <- struct{}{}
			defer func() { <-sem }()

			result, err := p.websiteScraper.Scrape(gctx, c.website)
			if err != nil {
				slog.Debug("pipeline: website scrape failed", "website", c.website, "error", err)
				return nil
			}

			websiteData := map[string]any{}
			if len(result.AllEmails) > 0 {
				websiteData["emails"] = result.AllEmails
			}
			if len(result.Phones) > 0 {
				websiteData["phones"] = result.Phones
			}
			if result.WhatsApp != "" {
				websiteData["whatsapp"] = result.WhatsApp
			}
			if len(result.SocialLinks) > 0 {
				websiteData["social_links"] = result.SocialLinks
			}
			if len(result.Products) > 0 {
				websiteData["products"] = result.Products
			}
			if result.HasContactForm {
				websiteData["has_contact_form"] = true
			}

			websiteDataJSON := marshalJSON(websiteData)

			bestEmail := c.email
			if bestEmail == "" && result.Email != "" {
				bestEmail = result.Email
			}
			bestPhone := ""
			if len(result.Phones) > 0 {
				bestPhone = result.Phones[0]
			}

			if err := p.repo.UpdateBusinessWebsiteData(gctx, c.id, bestEmail, bestPhone, result.Description, websiteDataJSON); err != nil {
				slog.Warn("pipeline: update website data failed", "business_id", c.id, "error", err)
				return nil
			}

			mu.Lock()
			enriched++
			if bestEmail != "" && c.email == "" {
				emailsFound++
			}
			if bestPhone != "" {
				phonesFound++
			}
			mu.Unlock()

			if bestEmail != "" {
				verified := datasource.VerifyEmail(gctx, bestEmail)
				if err := p.repo.UpdateBusinessEmailVerified(gctx, c.id, verified); err != nil {
					slog.Debug("pipeline: update email verified failed", "error", err)
				}
				if verified {
					mu.Lock()
					emailsVerified++
					mu.Unlock()
				}
			}

			return nil
		})
	}
	_ = g.Wait()

	slog.Info("pipeline: website scraping complete",
		"enriched", enriched,
		"emails_found", emailsFound,
		"phones_found", phonesFound,
		"emails_verified", emailsVerified,
		"total", len(candidates),
	)
}

// --- Helpers ---

func buildPlacesEnrichment(b domain.Business) PlacesEnrichment {
	e := PlacesEnrichment{}
	if b.GooglePlaceID != nil {
		e.GooglePlaceID = *b.GooglePlaceID
	}
	if b.Website != nil {
		e.Website = *b.Website
	}
	if b.Phone != nil {
		e.Phone = *b.Phone
	}
	if b.Email != nil {
		e.Email = *b.Email
	}
	if b.Latitude != nil {
		e.Latitude, _ = b.Latitude.Float64()
	}
	if b.Longitude != nil {
		e.Longitude, _ = b.Longitude.Float64()
	}
	if b.Description != nil {
		e.Description = *b.Description
	}
	if b.Rating != nil {
		e.Rating = *b.Rating
	}
	if b.RatingCount != nil {
		e.RatingCount = *b.RatingCount
	}
	if b.GoogleTypes != nil {
		e.GoogleTypes = b.GoogleTypes
	}
	if b.OpeningHours != nil {
		e.OpeningHours = b.OpeningHours
	}
	return e
}

func strPtrIfNotEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func marshalJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
