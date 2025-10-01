package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	echojwt "github.com/labstack/echo-jwt/v4"
	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"
	redis "xcentro.in/internal/adapters/cache/redis/client"
	"xcentro.in/internal/shared/config"
	"xcentro.in/internal/shared/logger"
	"xcentro.in/internal/shared/types"
)

type AuthService struct {
	config  *config.JWTSecretConfig
	rclient *redis.RedisClient
}

func NewAuthService(cfg *config.JWTSecretConfig, rclient *redis.RedisClient) *AuthService {
	return &AuthService{
		config:  cfg,
		rclient: rclient,
	}
}

func (s *AuthService) CreateAuthJWT(userid string) (string, error) {
	claim := jwt.RegisteredClaims{
		Subject:   userid,
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claim)
	stoken, err := token.SignedString([]byte(s.config.SecretKey))

	if err != nil {
		logger.Error("token signing failed: %v", err)
		return "", err
	}
	return stoken, nil
}

func (s *AuthService) ExtractAndValidateClaims(token *jwt.Token) (*jwt.RegisteredClaims, error) {
	//just need to check if token is expired or not for now
	if claims, ok := token.Claims.(*jwt.RegisteredClaims); ok && token.Valid {
		return claims, nil
	}
	return nil, nil
}

func (s *AuthService) parseToken(c echo.Context, auth string) (interface{}, error) {
	// The key lookup logic is defined as an inline function.
	keyFunc := func(token *jwt.Token) (interface{}, error) {
		// Verify the token's signing algorithm.
		if token.Method.Alg() != jwt.SigningMethodHS256.Name {
			return nil, fmt.Errorf("unexpected jwt signing method=%v", token.Header["alg"])
		}

		isrevoked, err := s.CheckIfTokenRevoked(c.Request().Context(), token.Claims.(*jwt.RegisteredClaims).Subject, auth)
		if err != nil {
			return nil, err
		}
		if isrevoked {
			return nil, fmt.Errorf("token is revoked")
		}

		return []byte(s.config.SecretKey), nil
	}

	// Parse the token.
	token, err := jwt.ParseWithClaims(auth, &jwt.RegisteredClaims{}, keyFunc)

	// Handle all errors by wrapping them in your custom ReplyError.
	if err != nil {
		return nil, types.ReplyError{Msg: err.Error()}
	}
	if !token.Valid {
		return nil, types.ReplyError{Msg: "invalid token"}
	}

	c.Set("userId", token.Claims.(*jwt.RegisteredClaims).Subject)

	return token, nil
}

func (s *AuthService) GetEchoAuthMiddleware() echo.MiddlewareFunc {
	return echojwt.WithConfig(echojwt.Config{
		ContextKey:     "token",
		ParseTokenFunc: s.parseToken,
	})
}

func (s *AuthService) HashPassword(password string) (string, error) {
	//TODO: need to add salt from .env
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashedPassword), nil
}

func (s *AuthService) ComparePassword(hashedPassword, password string) error {
	//TODO: need to add salt from .env
	return bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
}

func (s *AuthService) GetRedisClient() *redis.RedisClient {
	return s.rclient
}

func (s *AuthService) CheckIfTokenRevoked(ctx context.Context, userid, token string) (bool, error) {
	result := s.rclient.Client.HGet(ctx, "xcentro:user:"+userid+":token", token)
	if result.Err() != nil {
		return false, result.Err()
	}
	return result.Val() == "true", nil
}

func (s *AuthService) SetNewToken(ctx context.Context, userid string, token string) error {
	result := s.rclient.Client.HSet(ctx, "xcentro:user:"+userid+":token", token, false)
	return result.Err()
}
