package auth

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
	echojwt "github.com/labstack/echo-jwt/v4"
	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"
	redis "xcentro.in/internal/adapters/cache/redis/client"
	"xcentro.in/internal/shared/config"
	"xcentro.in/internal/shared/logger"
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

func (s *AuthService) GetEchoAuthMiddleware() echo.MiddlewareFunc {
	return echojwt.WithConfig(echojwt.Config{
		SigningKey: []byte(s.config.SecretKey),
		ContextKey: "token",
		NewClaimsFunc: func(c echo.Context) jwt.Claims {
			return &jwt.RegisteredClaims{}
		},
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
