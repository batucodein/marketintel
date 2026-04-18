package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/batuhan/marketintel/internal/domain"
)

type Service struct {
	users UserRepository
	jwt   *JWTManager
}

func NewService(users UserRepository, jwt *JWTManager) *Service {
	return &Service{users: users, jwt: jwt}
}

func (s *Service) Register(ctx context.Context, email, password string, companyName, homeCountry *string) (*TokenPair, error) {
	existing, _ := s.users.GetByEmail(ctx, email)
	if existing != nil {
		return nil, fmt.Errorf("%w: email already registered", domain.ErrConflict)
	}

	hashed, err := HashPassword(password)
	if err != nil {
		return nil, err
	}

	user := &domain.User{
		Email:             email,
		HashedPassword:    hashed,
		CompanyName:       companyName,
		HomeCountry:       homeCountry,
		SubscriptionTier:  "free",
		APICallsRemaining: 1000,
	}

	if err := s.users.Create(ctx, user); err != nil {
		return nil, fmt.Errorf("creating user: %w", err)
	}

	return s.jwt.GenerateTokenPair(user.ID)
}

func (s *Service) Login(ctx context.Context, email, password string) (*TokenPair, error) {
	user, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.ErrUnauthorized
		}
		return nil, err
	}

	if !VerifyPassword(password, user.HashedPassword) {
		return nil, domain.ErrUnauthorized
	}

	return s.jwt.GenerateTokenPair(user.ID)
}

func (s *Service) Refresh(ctx context.Context, refreshToken string) (*TokenPair, error) {
	userID, err := s.jwt.ValidateToken(refreshToken, "refresh")
	if err != nil {
		return nil, domain.ErrUnauthorized
	}

	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return nil, domain.ErrUnauthorized
	}

	return s.jwt.GenerateTokenPair(user.ID)
}

func (s *Service) GetUser(ctx context.Context, userID uuid.UUID) (*domain.User, error) {
	return s.users.GetByID(ctx, userID)
}

func (s *Service) UpdateProfile(ctx context.Context, userID uuid.UUID, companyName, homeCountry *string) (*domain.User, error) {
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	if companyName != nil {
		user.CompanyName = companyName
	}
	if homeCountry != nil {
		user.HomeCountry = homeCountry
	}

	if err := s.users.Update(ctx, user); err != nil {
		return nil, fmt.Errorf("updating user: %w", err)
	}

	return user, nil
}
