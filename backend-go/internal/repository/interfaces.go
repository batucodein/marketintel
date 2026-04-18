package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/batuhan/marketintel/internal/domain"
)

// UserRepo is owned by the auth module (internal/auth/repository.go).

type BusinessRepo interface {
	Create(ctx context.Context, b *domain.Business) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Business, error)
	GetByGooglePlaceID(ctx context.Context, placeID string) (*domain.Business, error)
	Update(ctx context.Context, b *domain.Business) error
	ListByMarket(ctx context.Context, marketID uuid.UUID, page, pageSize int) ([]domain.BusinessWithRelevance, error)
	CountByMarket(ctx context.Context, marketID uuid.UUID) (int, error)
}

type BusinessMarketRepo interface {
	Upsert(ctx context.Context, bm *domain.BusinessMarket) error
	UpdateRelevanceScore(ctx context.Context, businessID, marketID uuid.UUID, score decimal.Decimal) error
}

type MarketRepo interface {
	Create(ctx context.Context, m *domain.Market) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Market, error)
	GetByLocationProduct(ctx context.Context, countryCode string, city *string, productCategoryID *uuid.UUID) (*domain.Market, error)
	List(ctx context.Context) ([]domain.Market, error)
	Update(ctx context.Context, m *domain.Market) error
}

type MarketAnalysisRepo interface {
	Create(ctx context.Context, a *domain.MarketAnalysis) error
	GetLatestByMarket(ctx context.Context, marketID uuid.UUID) (*domain.MarketAnalysis, error)
}

type MarketRankingRepo interface {
	Create(ctx context.Context, r *domain.MarketRanking) error
	ListByUserAndCategory(ctx context.Context, userID, categoryID uuid.UUID) ([]domain.MarketRanking, error)
}

type SearchRepo interface {
	Create(ctx context.Context, s *domain.Search) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Search, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status string, resultCount *int) error
}

type LeadScoreRepo interface {
	Create(ctx context.Context, ls *domain.LeadScore) error
	GetByBusinessAndMarket(ctx context.Context, businessID, marketID uuid.UUID) (*domain.LeadScore, error)
	ListByMarketAndUser(ctx context.Context, marketID, userID uuid.UUID, minScore int, page, pageSize int) ([]domain.LeadScore, error)
	CountByUser(ctx context.Context, userID uuid.UUID) (int, error)
}

type TradeFlowRepo interface {
	Create(ctx context.Context, tf *domain.TradeFlow) error
	ListByHSCode(ctx context.Context, hsCode string) ([]domain.TradeFlow, error)
	ListByPartner(ctx context.Context, partnerCode, hsCode string) ([]domain.TradeFlow, error)
}

type AIRequestLogRepo interface {
	Create(ctx context.Context, log *domain.AIRequestLog) error
	GetCostSummary(ctx context.Context, userID uuid.UUID) (*domain.AICostSummary, error)
}

type CompetitorRepo interface {
	Create(ctx context.Context, c *domain.Competitor) error
	ListByMarket(ctx context.Context, marketID uuid.UUID) ([]domain.Competitor, error)
}

type ProductCategoryRepo interface {
	Create(ctx context.Context, pc *domain.ProductCategory) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.ProductCategory, error)
	GetBySlug(ctx context.Context, slug string) (*domain.ProductCategory, error)
	GetByHSCode(ctx context.Context, hsCode string) (*domain.ProductCategory, error)
}
