package service

import (
	"context"
	"fmt"

	"github.com/mumumio1/coldy/services/users/internal/repository"
	"go.uber.org/zap"
)

// UserService handles user business logic
type UserService struct {
	repo        *repository.UserRepository
	authService *AuthService
	logger      *zap.Logger
}

// NewUserService creates a new user service
func NewUserService(repo *repository.UserRepository, authService *AuthService, logger *zap.Logger) *UserService {
	return &UserService{
		repo:        repo,
		authService: authService,
		logger:      logger,
	}
}

// Register registers a new user
func (s *UserService) Register(ctx context.Context, email, password, fullName, phone string) (*repository.User, string, string, error) {
	// Check if user exists
	existing, err := s.repo.GetByEmail(ctx, email)
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to check existing user: %w", err)
	}
	if existing != nil {
		return nil, "", "", fmt.Errorf("user with email %s already exists", email)
	}

	// Hash password
	passwordHash, err := s.authService.HashPassword(ctx, password)
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to hash password: %w", err)
	}

	// Create user
	user := &repository.User{
		Email:        email,
		PasswordHash: passwordHash,
		FullName:     fullName,
		Phone:        phone,
	}

	if err := s.repo.Create(ctx, user); err != nil {
		return nil, "", "", fmt.Errorf("failed to create user: %w", err)
	}

	// Generate tokens
	accessToken, err := s.authService.GenerateAccessToken(ctx, user.ID, user.Email)
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to generate access token: %w", err)
	}

	refreshToken, err := s.authService.GenerateRefreshToken(ctx, user.ID, user.Email)
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to generate refresh token: %w", err)
	}

	s.logger.Info("user registered",
		zap.String("user_id", user.ID),
		zap.String("email", user.Email),
	)

	return user, accessToken, refreshToken, nil
}

// Login authenticates a user
func (s *UserService) Login(ctx context.Context, email, password string) (*repository.User, string, string, error) {
	// Get user by email
	user, err := s.repo.GetByEmail(ctx, email)
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to get user: %w", err)
	}
	if user == nil {
		return nil, "", "", fmt.Errorf("invalid credentials")
	}

	// Verify password
	if err := s.authService.VerifyPassword(ctx, password, user.PasswordHash); err != nil {
		return nil, "", "", fmt.Errorf("invalid credentials")
	}

	// Generate tokens
	accessToken, err := s.authService.GenerateAccessToken(ctx, user.ID, user.Email)
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to generate access token: %w", err)
	}

	refreshToken, err := s.authService.GenerateRefreshToken(ctx, user.ID, user.Email)
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to generate refresh token: %w", err)
	}

	s.logger.Info("user logged in",
		zap.String("user_id", user.ID),
		zap.String("email", user.Email),
	)

	return user, accessToken, refreshToken, nil
}

// GetUser retrieves a user by ID
func (s *UserService) GetUser(ctx context.Context, userID string) (*repository.User, error) {
	user, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	if user == nil {
		return nil, fmt.Errorf("user not found")
	}
	return user, nil
}

// UpdateUser updates a user
func (s *UserService) UpdateUser(ctx context.Context, userID, fullName, phone string) (*repository.User, error) {
	user, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	if user == nil {
		return nil, fmt.Errorf("user not found")
	}

	user.FullName = fullName
	user.Phone = phone

	if err := s.repo.Update(ctx, user); err != nil {
		return nil, fmt.Errorf("failed to update user: %w", err)
	}

	s.logger.Info("user updated", zap.String("user_id", user.ID))

	return user, nil
}

// ListUsers lists users with pagination
func (s *UserService) ListUsers(ctx context.Context, limit int, cursor string) ([]*repository.User, string, bool, error) {
	users, nextCursor, err := s.repo.List(ctx, limit, cursor)
	if err != nil {
		return nil, "", false, fmt.Errorf("failed to list users: %w", err)
	}

	hasMore := nextCursor != ""

	return users, nextCursor, hasMore, nil
}
