package payment

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap/zaptest"
)

// Mocks
type MockRepository struct {
	mock.Mock
}

func (m *MockRepository) Save(ctx context.Context, payment *Payment) error {
	args := m.Called(ctx, payment)
	return args.Error(0)
}

func (m *MockRepository) GetByID(ctx context.Context, id uuid.UUID) (*Payment, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Payment), args.Error(1)
}

func (m *MockRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status PaymentStatus) error {
	args := m.Called(ctx, id, status)
	return args.Error(0)
}

type MockWalletService struct {
	mock.Mock
}

func (m *MockWalletService) ValidateBalance(ctx context.Context, userID uuid.UUID, amount int64, currency string) error {
	args := m.Called(ctx, userID, amount, currency)
	return args.Error(0)
}

func (m *MockWalletService) DeductBalance(ctx context.Context, userID uuid.UUID, amount int64, currency string, paymentID uuid.UUID) error {
	args := m.Called(ctx, userID, amount, currency, paymentID)
	return args.Error(0)
}

func (m *MockWalletService) RefundBalance(ctx context.Context, userID uuid.UUID, amount int64, currency string, paymentID uuid.UUID, reason string) error {
	args := m.Called(ctx, userID, amount, currency, paymentID, reason)
	return args.Error(0)
}

type MockGatewayService struct {
	mock.Mock
}

func (m *MockGatewayService) ProcessPayment(ctx context.Context, payment *Payment) (string, error) {
	args := m.Called(ctx, payment)
	return args.String(0), args.Error(1)
}

type MockEventPublisher struct {
	mock.Mock
}

func (m *MockEventPublisher) Publish(ctx context.Context, event interface{}) error {
	args := m.Called(ctx, event)
	return args.Error(0)
}

func (m *MockEventPublisher) PublishBatch(ctx context.Context, events []interface{}) error {
	args := m.Called(ctx, events)
	return args.Error(0)
}

func (m *MockEventPublisher) Close() error {
	args := m.Called()
	return args.Error(0)
}

func TestService_ProcessPayment_Success(t *testing.T) {
	// Setup
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	mockRepo := &MockRepository{}
	mockWallet := &MockWalletService{}
	mockGateway := &MockGatewayService{}
	mockPublisher := &MockEventPublisher{}

	service := NewService(mockRepo, mockWallet, mockGateway, mockPublisher, logger)

	userID := uuid.New()
	req := &PaymentRequest{
		UserID:      userID,
		Amount:      10000,
		Currency:    "USD",
		ServiceID:   "test_service",
		Description: "Test payment",
	}

	// Mock expectations
	mockRepo.On("Save", ctx, mock.AnythingOfType("*payment.Payment")).Return(nil)
	mockPublisher.On("Publish", ctx, mock.AnythingOfType("events.PaymentInitiatedEvent")).Return(nil)
	mockWallet.On("ValidateBalance", ctx, userID, int64(10000), "USD").Return(nil)
	mockWallet.On("DeductBalance", ctx, userID, int64(10000), "USD", mock.AnythingOfType("uuid.UUID")).Return(nil)
	mockRepo.On("UpdateStatus", ctx, mock.AnythingOfType("uuid.UUID"), StatusProcessing).Return(nil)
	
	// Mock gateway call for the goroutine (uses context.Background())
	mockGateway.On("ProcessPayment", mock.Anything, mock.AnythingOfType("*payment.Payment")).Return("gw_test_12345", nil)
	mockRepo.On("UpdateStatus", mock.Anything, mock.AnythingOfType("uuid.UUID"), StatusCompleted).Return(nil)
	mockPublisher.On("Publish", mock.Anything, mock.AnythingOfType("events.PaymentCompletedEvent")).Return(nil)

	// Execute
	payment, err := service.ProcessPayment(ctx, req)

	// Give goroutine time to complete
	time.Sleep(50 * time.Millisecond)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, payment)
	assert.Equal(t, userID, payment.UserID)
	assert.Equal(t, int64(10000), payment.Amount)
	assert.Equal(t, "USD", payment.Currency)
	// The returned payment should have status "processing" - goroutine updates DB separately
	assert.Equal(t, StatusProcessing, payment.Status)

	// Verify mocks
	mockRepo.AssertExpectations(t)
	mockWallet.AssertExpectations(t)
	mockPublisher.AssertExpectations(t)
	mockGateway.AssertExpectations(t)
}

func TestService_ProcessPayment_InsufficientBalance(t *testing.T) {
	// Setup
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	mockRepo := &MockRepository{}
	mockWallet := &MockWalletService{}
	mockGateway := &MockGatewayService{}
	mockPublisher := &MockEventPublisher{}

	service := NewService(mockRepo, mockWallet, mockGateway, mockPublisher, logger)

	userID := uuid.New()
	req := &PaymentRequest{
		UserID:      userID,
		Amount:      10000,
		Currency:    "USD",
		ServiceID:   "test_service",
		Description: "Test payment",
	}

	// Mock expectations
	mockRepo.On("Save", ctx, mock.AnythingOfType("*payment.Payment")).Return(nil)
	mockPublisher.On("Publish", ctx, mock.AnythingOfType("events.PaymentInitiatedEvent")).Return(nil)
	mockWallet.On("ValidateBalance", ctx, userID, int64(10000), "USD").Return(errors.New("insufficient balance"))
	mockRepo.On("UpdateStatus", ctx, mock.AnythingOfType("uuid.UUID"), StatusFailed).Return(nil)
	mockPublisher.On("Publish", ctx, mock.AnythingOfType("events.PaymentFailedEvent")).Return(nil)
	
	// Add gateway mock expectation to handle potential race condition
	mockGateway.On("ProcessPayment", mock.Anything, mock.AnythingOfType("*payment.Payment")).Return("", errors.New("should not be called")).Maybe()

	// Execute
	payment, err := service.ProcessPayment(ctx, req)

	// Assert
	assert.Error(t, err)
	assert.NotNil(t, payment)
	assert.Equal(t, StatusFailed, payment.Status)
	assert.Contains(t, err.Error(), "insufficient balance")

	// Verify mocks
	mockRepo.AssertExpectations(t)
	mockWallet.AssertExpectations(t)
	mockPublisher.AssertExpectations(t)
}

func TestService_ProcessPayment_DeductionFailed(t *testing.T) {
	// Setup
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	mockRepo := &MockRepository{}
	mockWallet := &MockWalletService{}
	mockGateway := &MockGatewayService{}
	mockPublisher := &MockEventPublisher{}

	service := NewService(mockRepo, mockWallet, mockGateway, mockPublisher, logger)

	userID := uuid.New()
	req := &PaymentRequest{
		UserID:      userID,
		Amount:      10000,
		Currency:    "USD",
		ServiceID:   "test_service",
		Description: "Test payment",
	}

	// Mock expectations
	mockRepo.On("Save", ctx, mock.AnythingOfType("*payment.Payment")).Return(nil)
	mockPublisher.On("Publish", ctx, mock.AnythingOfType("events.PaymentInitiatedEvent")).Return(nil)
	mockWallet.On("ValidateBalance", ctx, userID, int64(10000), "USD").Return(nil)
	mockWallet.On("DeductBalance", ctx, userID, int64(10000), "USD", mock.AnythingOfType("uuid.UUID")).Return(errors.New("deduction failed"))
	mockRepo.On("UpdateStatus", ctx, mock.AnythingOfType("uuid.UUID"), StatusFailed).Return(nil)
	mockPublisher.On("Publish", ctx, mock.AnythingOfType("events.PaymentFailedEvent")).Return(nil)

	// Execute
	payment, err := service.ProcessPayment(ctx, req)

	// Assert
	assert.Error(t, err)
	assert.NotNil(t, payment)
	assert.Equal(t, StatusFailed, payment.Status)
	assert.Contains(t, err.Error(), "failed to deduct balance")

	// Verify mocks
	mockRepo.AssertExpectations(t)
	mockWallet.AssertExpectations(t)
	mockPublisher.AssertExpectations(t)
}

func TestService_GetPayment_Success(t *testing.T) {
	// Setup
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	mockRepo := &MockRepository{}
	mockWallet := &MockWalletService{}
	mockGateway := &MockGatewayService{}
	mockPublisher := &MockEventPublisher{}

	service := NewService(mockRepo, mockWallet, mockGateway, mockPublisher, logger)

	paymentID := uuid.New()
	expectedPayment := &Payment{
		ID:       paymentID,
		UserID:   uuid.New(),
		Amount:   10000,
		Currency: "USD",
		Status:   StatusCompleted,
	}

	// Mock expectations
	mockRepo.On("GetByID", ctx, paymentID).Return(expectedPayment, nil)

	// Execute
	payment, err := service.GetPayment(ctx, paymentID)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, expectedPayment, payment)

	// Verify mocks
	mockRepo.AssertExpectations(t)
}

func TestService_GetPayment_NotFound(t *testing.T) {
	// Setup
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	mockRepo := &MockRepository{}
	mockWallet := &MockWalletService{}
	mockGateway := &MockGatewayService{}
	mockPublisher := &MockEventPublisher{}

	service := NewService(mockRepo, mockWallet, mockGateway, mockPublisher, logger)

	paymentID := uuid.New()

	// Mock expectations
	mockRepo.On("GetByID", ctx, paymentID).Return(nil, errors.New("not found"))

	// Execute
	payment, err := service.GetPayment(ctx, paymentID)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, payment)

	// Verify mocks
	mockRepo.AssertExpectations(t)
}

// Benchmark tests
func BenchmarkService_ProcessPayment(b *testing.B) {
	ctx := context.Background()
	logger := zaptest.NewLogger(b)

	mockRepo := &MockRepository{}
	mockWallet := &MockWalletService{}
	mockGateway := &MockGatewayService{}
	mockPublisher := &MockEventPublisher{}

	service := NewService(mockRepo, mockWallet, mockGateway, mockPublisher, logger)

	userID := uuid.New()
	req := &PaymentRequest{
		UserID:      userID,
		Amount:      10000,
		Currency:    "USD",
		ServiceID:   "test_service",
		Description: "Test payment",
	}

	// Setup mocks for benchmark
	mockRepo.On("Save", ctx, mock.AnythingOfType("*payment.Payment")).Return(nil)
	mockPublisher.On("Publish", ctx, mock.AnythingOfType("events.PaymentInitiatedEvent")).Return(nil)
	mockWallet.On("ValidateBalance", ctx, userID, int64(10000), "USD").Return(nil)
	mockWallet.On("DeductBalance", ctx, userID, int64(10000), "USD", mock.AnythingOfType("uuid.UUID")).Return(nil)
	mockRepo.On("UpdateStatus", ctx, mock.AnythingOfType("uuid.UUID"), StatusProcessing).Return(nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		service.ProcessPayment(ctx, req)
	}
}
