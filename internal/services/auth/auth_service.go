package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	echojwt "github.com/labstack/echo-jwt/v4"
	"github.com/labstack/echo/v4"
	goredis "github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
	redis "xcentro.in/internal/adapters/cache/redis/client"
	"xcentro.in/internal/adapters/cache/redis/keys"
	"xcentro.in/internal/core/auth"
	"xcentro.in/internal/core/business"
	"xcentro.in/internal/core/shared"
	"xcentro.in/internal/core/user"
	"xcentro.in/internal/shared/config"
	"xcentro.in/internal/shared/logger"
	"xcentro.in/internal/shared/types"
)

type AuthService struct {
	config          *config.JWTSecretConfig
	rclient         *redis.RedisClient
	userService     user.UserService
	businessService business.BusinessService
}

func NewAuthService(
	cfg *config.JWTSecretConfig,
	rclient *redis.RedisClient,
	userService user.UserService,
	businessService business.BusinessService,
) *AuthService {
	return &AuthService{
		config:          cfg,
		rclient:         rclient,
		userService:     userService,
		businessService: businessService,
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
	// Parse the token first to get claims for debugging
	token, err := jwt.ParseWithClaims(auth, &jwt.RegisteredClaims{}, func(token *jwt.Token) (interface{}, error) {
		// Verify the token's signing algorithm.
		if token.Method.Alg() != jwt.SigningMethodHS256.Name {
			logger.Error("ERROR: unexpected jwt signing method=%v\n", token.Header["alg"])
			return nil, fmt.Errorf("unexpected jwt signing method=%v", token.Header["alg"])
		}

		// Get subject from claims for debugging
		claims := token.Claims.(*jwt.RegisteredClaims)
		fmt.Printf("Token subject: %s\n", claims.Subject)

		// Check token revocation
		isrevoked, err := s.CheckIfTokenRevoked(c.Request().Context(), claims.Subject, auth)
		if err != nil {
			logger.Error("ERROR: checking if token is revoked: %v\n", err)
			return nil, err
		}
		if isrevoked {
			logger.Error("ERROR: token is revoked\n")
			return nil, fmt.Errorf("token is revoked")
		}

		logger.Info("Token validation successful, returning secret key\n")
		return []byte(s.config.SecretKey), nil
	})

	// Handle parsing errors
	if err != nil {
		fmt.Printf("ERROR: parsing jwt token: %v\n", err)
		return nil, types.ReplyError{Msg: err.Error()}
	}
	if !token.Valid {
		fmt.Printf("ERROR: invalid jwt token\n")
		return nil, types.ReplyError{Msg: "invalid token"}
	}

	// Set userId in context
	userId := token.Claims.(*jwt.RegisteredClaims).Subject
	fmt.Printf("Setting userId in context: %s\n", userId)
	c.Set("userId", userId)

	fmt.Printf("parseToken completed successfully\n")
	return token, nil
}

func (s *AuthService) GetEchoAuthMiddleware() echo.MiddlewareFunc {
	return echojwt.WithConfig(echojwt.Config{
		ContextKey:     "token",
		ParseTokenFunc: s.parseToken,
		ErrorHandler: func(c echo.Context, err error) error {
			fmt.Printf("JWT Middleware Error: %v\n", err)
			return c.JSON(401, map[string]any{"message": "unauthorized", "data": nil})
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

func (s *AuthService) CheckIfTokenRevoked(ctx context.Context, userid, token string) (bool, error) {
	key := keys.User.Token(userid)
	result := s.rclient.Client.HGet(ctx, key, token)
	if result.Err() != nil {
		return false, result.Err()
	}
	return result.Val() == "true", nil
}

func (s *AuthService) SetNewToken(ctx context.Context, userid string, token string) error {
	key := keys.User.Token(userid)
	result := s.rclient.Client.HSet(ctx, key, token, false)
	return result.Err()
}

// Signup handles complete user registration flow
// Returns SignupResponse with minimal user info
func (s *AuthService) Signup(ctx context.Context, email, password string) (*auth.SignupResponse, error) {
	logger.Info("Processing signup for email: %s", email)

	// Validate input
	if email == "" || password == "" {
		return nil, fmt.Errorf("email and password are required")
	}

	// Hash password
	hashedPassword, err := s.HashPassword(password)
	if err != nil {
		logger.Errorf("Failed to hash password: %v", err)
		return nil, fmt.Errorf("failed to process password")
	}

	// Create user
	authPayload := auth.AuthPayload{
		Email:    email,
		Password: hashedPassword,
	}

	createdUser, err := s.userService.CreateUser(ctx, authPayload)
	if err != nil {
		logger.Errorf("Failed to create user: %v", err)
		return nil, fmt.Errorf("failed to create user")
	}

	// Send email verification (non-blocking)
	go func() {
		bgCtx := context.Background()
		if err := s.userService.SendEmailVerification(bgCtx, createdUser.Id); err != nil {
			logger.Errorf("Failed to send email verification: %v", err)
			// Don't fail signup if email sending fails
		}
	}()

	return &auth.SignupResponse{
		UserID:  createdUser.Id,
		Message: "User created successfully. Please check your email for verification link.",
	}, nil
}

// Login handles complete login flow with caching
// Returns LoginResponse with minimal data: token, kycNeeded, kybNeeded
func (s *AuthService) Login(ctx context.Context, email, password string) (*auth.LoginResponse, error) {
	logger.Info("Processing login for email: %s", email)

	// Validate input
	if email == "" || password == "" {
		return nil, fmt.Errorf("email and password are required")
	}

	// Get user by email (cached)
	userObj, err := s.userService.GetUserByEmail(ctx, email)
	if err != nil {
		logger.Errorf("Failed to get user by email: %v", err)
		return nil, fmt.Errorf("invalid email or password")
	}

	// Check email verification
	if !userObj.EmailVerified {
		logger.Error("Email not verified for user: %s", userObj.Id)
		return nil, fmt.Errorf("email not verified. Please verify your email before logging in")
	}

	// Verify password
	if err := s.ComparePassword(userObj.PwdHash, password); err != nil {
		logger.Error("Invalid password for user: %s", userObj.Id)
		return nil, fmt.Errorf("invalid email or password")
	}

	// Create JWT token
	token, err := s.CreateAuthJWT(userObj.Id)
	if err != nil {
		logger.Errorf("Failed to create token: %v", err)
		return nil, fmt.Errorf("failed to create authentication token")
	}

	// Store token in Redis
	if err := s.SetNewToken(ctx, userObj.Id, token); err != nil {
		logger.Errorf("Failed to store token: %v", err)
		return nil, fmt.Errorf("failed to store authentication token")
	}

	// Get onboarding status (cached)
	onboardingStatus := s.getOnboardingStatus(ctx, userObj)

	return &auth.LoginResponse{
		Token:     token,
		KycNeeded: onboardingStatus.KycNeeded,
		KybNeeded: onboardingStatus.KybNeeded,
	}, nil
}

// getOnboardingStatus determines KYC and KYB status with caching
// Returns cached result if available, otherwise computes and caches
func (s *AuthService) getOnboardingStatus(ctx context.Context, userObj *user.User) *auth.OnboardingStatusCache {
	// Try cache first
	cacheKey := keys.User.OnboardingStatus(userObj.Id)
	var cached auth.OnboardingStatusCache
	err := s.rclient.GetJSON(ctx, cacheKey, &cached)

	if err == nil {
		// Cache hit
		logger.Info("Onboarding status cache hit for user: %s", userObj.Id)
		return &cached
	}

	// Cache miss - compute status
	if !errors.Is(err, goredis.Nil) {
		logger.Warnf("Redis cache error for onboarding status (continuing): %v", err)
	}

	// KYC Logic: Check both kyc_needed flag AND kyc_status
	// - If kyc_needed = false → user doesn't need KYC (invited user or exempted)
	// - If kyc_needed = true → check kyc_status:
	//   - If status is completed or verified → KYC done
	//   - Otherwise → KYC still needed
	kycNeeded := false
	if userObj.KycNeeded {
		// User requires KYC - check if it's complete
		if userObj.KycStatus != shared.KYCCompleted && userObj.KycStatus != shared.KYCVerified {
			kycNeeded = true
		}
	}

	// KYB Logic: Check business onboarding status
	// - Get business relations to determine if KYB is needed
	kybNeeded := false
	onboardingStatus, err := s.businessService.GetOnboardingStatus(ctx, userObj.Id)
	if err != nil {
		logger.Warnf("Failed to get business onboarding status: %v", err)
		// If user has business relations but we can't get status, assume KYB needed
		kybNeeded = true
	} else if onboardingStatus != nil && onboardingStatus.KYBInitiated {
		// User has started KYB - check if it's complete
		kybNeeded = onboardingStatus.KYBStatus != shared.KYBCompleted && onboardingStatus.KYBStatus != shared.KYBVerified
	} else if onboardingStatus != nil && onboardingStatus.KYBStatus == shared.KYBNotStarted {
		// User has business relation but hasn't started KYB
		kybNeeded = true
	}
	// If onboardingStatus is nil or has no business relations, kybNeeded remains false

	result := &auth.OnboardingStatusCache{
		KycNeeded: kycNeeded,
		KybNeeded: kybNeeded,
	}

	// Cache result asynchronously
	go func() {
		bgCtx := context.Background()
		if err := s.rclient.SetJSON(bgCtx, cacheKey, result, keys.TTLOnboardingStatus); err != nil {
			logger.Warnf("Failed to cache onboarding status: %v", err)
		}
	}()

	return result
}
