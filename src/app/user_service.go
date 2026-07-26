package app

import (
	"App/src/domain"
	"App/src/pkg/logger"
	"App/src/ports"
	"context"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// UserService handles user-related business logic.
type UserService struct {
	users  ports.UserRepository
	logger logger.Logger
}

// NewUserService creates a new UserService.
func NewUserService(users ports.UserRepository, log logger.Logger) *UserService {
	return &UserService{
		users:  users,
		logger: log.WithComponent("user_service"),
	}
}

// Register creates a new user after validating inputs and checking for duplicates.
func (s *UserService) Register(ctx context.Context, username, email, phone, password string) (*domain.User, error) {
	if username == "" || email == "" || phone == "" || password == "" {
		return nil, domain.ErrInvalidInput
	}
	if len(password) < 6 {
		return nil, fmt.Errorf("%w: password must be at least 6 characters", domain.ErrInvalidInput)
	}

	exists, err := s.users.CheckDuplicate(ctx, username, email, phone)
	if err != nil {
		return nil, fmt.Errorf("checking duplicates: %w", err)
	}
	if exists {
		return nil, domain.ErrDuplicateResource
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hashing password: %w", err)
	}

	user, err := s.users.Create(ctx, username, email, phone, string(hashed))
	if err != nil {
		return nil, fmt.Errorf("creating user: %w", err)
	}

	s.logger.Info().Str("username", username).Msg("User registered")
	return user, nil
}

// Authenticate validates credentials and returns the user.
func (s *UserService) Authenticate(ctx context.Context, username, email, password string) (*domain.User, error) {
	var user *domain.User
	var err error
	if username != "" {
		user, err = s.users.GetByUsername(ctx, username)
	}
	if user == nil && email != "" {
		user, err = s.users.GetByUsername(ctx, email) // try username field first
		if user == nil {
			user, err = s.users.GetByEmail(ctx, email)
		}
	}
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, domain.ErrUnauthorized
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, domain.ErrUnauthorized
	}
	return user, nil
}

// UpdatePassword changes a user's password.
func (s *UserService) UpdatePassword(ctx context.Context, userID int, newPassword string) error {
	if len(newPassword) < 6 {
		return fmt.Errorf("%w: password must be at least 6 characters", domain.ErrInvalidInput)
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.users.UpdatePassword(ctx, userID, string(hashed))
}

// UpdatePhone changes a user's phone number after checking for duplicates.
func (s *UserService) UpdatePhone(ctx context.Context, userID int, newPhone string) error {
	if newPhone == "" {
		return fmt.Errorf("%w: phone cannot be empty", domain.ErrInvalidInput)
	}
	taken, err := s.users.CheckPhoneTaken(ctx, newPhone, userID)
	if err != nil {
		return err
	}
	if taken {
		return fmt.Errorf("%w: phone number already registered", domain.ErrDuplicateResource)
	}
	return s.users.UpdatePhone(ctx, userID, newPhone)
}

// GetByID returns a user by ID.
func (s *UserService) GetByID(ctx context.Context, id int) (*domain.User, error) {
	return s.users.GetByID(ctx, id)
}

// GetByUsername returns a user by username.
func (s *UserService) GetByUsername(ctx context.Context, username string) (*domain.User, error) {
	return s.users.GetByUsername(ctx, username)
}

// ListAll returns all users (admin only).
func (s *UserService) ListAll(ctx context.Context, limit string, offset string) ([]domain.User, error) {
	return s.users.ListAll(ctx, limit, offset)
}

// Delete removes a user by ID.
func (s *UserService) Delete(ctx context.Context, id int) error {
	return s.users.Delete(ctx, id)
}
