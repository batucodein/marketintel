package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
}

type Claims struct {
	jwt.RegisteredClaims
	TokenType string `json:"type"`
}

type JWTManager struct {
	secret             []byte
	accessExpireMinutes int
	refreshExpireDays   int
}

func NewJWTManager(secret string, accessMinutes, refreshDays int) *JWTManager {
	return &JWTManager{
		secret:             []byte(secret),
		accessExpireMinutes: accessMinutes,
		refreshExpireDays:   refreshDays,
	}
}

func (m *JWTManager) GenerateTokenPair(userID uuid.UUID) (*TokenPair, error) {
	access, err := m.createToken(userID, "access", time.Duration(m.accessExpireMinutes)*time.Minute)
	if err != nil {
		return nil, fmt.Errorf("creating access token: %w", err)
	}

	refresh, err := m.createToken(userID, "refresh", time.Duration(m.refreshExpireDays)*24*time.Hour)
	if err != nil {
		return nil, fmt.Errorf("creating refresh token: %w", err)
	}

	return &TokenPair{
		AccessToken:  access,
		RefreshToken: refresh,
		TokenType:    "bearer",
	}, nil
}

func (m *JWTManager) ValidateToken(tokenStr string, expectedType string) (uuid.UUID, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return m.secret, nil
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("parsing token: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return uuid.Nil, fmt.Errorf("invalid token claims")
	}

	if claims.TokenType != expectedType {
		return uuid.Nil, fmt.Errorf("expected %s token, got %s", expectedType, claims.TokenType)
	}

	userID, err := uuid.Parse(claims.Subject)
	if err != nil {
		return uuid.Nil, fmt.Errorf("parsing user id from token: %w", err)
	}

	return userID, nil
}

func (m *JWTManager) createToken(userID uuid.UUID, tokenType string, expiry time.Duration) (string, error) {
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(expiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
		TokenType: tokenType,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.secret)
}
