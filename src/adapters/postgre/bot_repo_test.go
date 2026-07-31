package postgre

import (
	"App/src/domain"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockBotRepo implementa la interfaz ports.BotRepository
type MockBotRepo struct {
	mock.Mock
}

func (m *MockBotRepo) GetByID(ctx context.Context, id int) (*domain.Bot, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Bot), args.Error(1)
}

func (m *MockBotRepo) GetByUser(ctx context.Context, userID int) ([]domain.Bot, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).([]domain.Bot), args.Error(1)
}

func (m *MockBotRepo) GetAll(ctx context.Context) ([]domain.Bot, error) {
	args := m.Called(ctx)
	return args.Get(0).([]domain.Bot), args.Error(1)
}

func (m *MockBotRepo) Create(ctx context.Context, userID int, sessionFile, paymentStatus string) (int, error) {
	args := m.Called(ctx, userID, sessionFile, paymentStatus)
	return args.Int(0), args.Error(1)
}

func (m *MockBotRepo) UpdateBlocked(ctx context.Context, id int, blocked bool) error {
	return m.Called(ctx, id, blocked).Error(0)
}

func (m *MockBotRepo) UpdatePaymentStatus(ctx context.Context, id int, status string) error {
	return m.Called(ctx, id, status).Error(0)
}

func (m *MockBotRepo) UpdateSessionFile(ctx context.Context, id int, file string) error {
	return m.Called(ctx, id, file).Error(0)
}

func (m *MockBotRepo) Delete(ctx context.Context, id int) error {
	return m.Called(ctx, id).Error(0)
}

func (m *MockBotRepo) CountByUser(ctx context.Context, userID int) (int, error) {
	args := m.Called(ctx, userID)
	return args.Int(0), args.Error(1)
}

func TestBotRepo_Create_Mock(t *testing.T) {
	mockRepo := new(MockBotRepo)
	expectedBotID := 1
	mockRepo.On("Create", mock.Anything, 1, "session.db", "free").Return(expectedBotID, nil)

	botID, err := mockRepo.Create(context.Background(), 1, "session.db", "free")
	assert.NoError(t, err)
	assert.Equal(t, expectedBotID, botID)
	mockRepo.AssertExpectations(t)
}
