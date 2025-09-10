package wallet

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap/zaptest"
)

// Mock Repository
type MockWalletRepository struct {
	mock.Mock
}

func (m *MockWalletRepository) GetWalletByUserID(ctx context.Context, userID uuid.UUID, currency string) (*Wallet, error) {
	args := m.Called(ctx, userID, currency)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Wallet), args.Error(1)
}

func (m *MockWalletRepository) UpdateBalance(ctx context.Context, walletID uuid.UUID, newBalance int64, version int) error {
	args := m.Called(ctx, walletID, newBalance, version)
	return args.Error(0)
}

func (m *MockWalletRepository) CreateTransaction(ctx context.Context, txn *Transaction) error {
	args := m.Called(ctx, txn)
	return args.Error(0)
}

func (m *MockWalletRepository) GetTransactionsByWallet(ctx context.Context, walletID uuid.UUID) ([]*Transaction, error) {
	args := m.Called(ctx, walletID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*Transaction), args.Error(1)
}

// Mock Event Publisher
type MockWalletEventPublisher struct {
	mock.Mock
}

func (m *MockWalletEventPublisher) Publish(ctx context.Context, event interface{}) error {
	args := m.Called(ctx, event)
	return args.Error(0)
}

func (m *MockWalletEventPublisher) PublishBatch(ctx context.Context, events []interface{}) error {
	args := m.Called(ctx, events)
	return args.Error(0)
}

func (m *MockWalletEventPublisher) Close() error {
	args := m.Called()
	return args.Error(0)
}

func TestService_ValidateBalance_Success(t *testing.T) {
	// Setup
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	mockRepo := &MockWalletRepository{}
	mockPublisher := &MockWalletEventPublisher{}

	service := NewService(mockRepo, mockPublisher, logger)

	userID := uuid.New()
	walletID := uuid.New()
	wallet := &Wallet{
		ID:       walletID,
		UserID:   userID,
		Balance:  50000,
		Currency: "USD",
		Version:  1,
	}

	// Mock expectations
	mockRepo.On("GetWalletByUserID", ctx, userID, "USD").Return(wallet, nil)
	mockPublisher.On("Publish", ctx, mock.AnythingOfType("events.BaseEvent")).Return(nil)

	// Execute
	err := service.ValidateBalance(ctx, userID, 10000, "USD")

	// Assert
	assert.NoError(t, err)

	// Verify mocks
	mockRepo.AssertExpectations(t)
	mockPublisher.AssertExpectations(t)
}

func TestService_ValidateBalance_InsufficientFunds(t *testing.T) {
	// Setup
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	mockRepo := &MockWalletRepository{}
	mockPublisher := &MockWalletEventPublisher{}

	service := NewService(mockRepo, mockPublisher, logger)

	userID := uuid.New()
	walletID := uuid.New()
	wallet := &Wallet{
		ID:       walletID,
		UserID:   userID,
		Balance:  5000,
		Currency: "USD",
		Version:  1,
	}

	// Mock expectations
	mockRepo.On("GetWalletByUserID", ctx, userID, "USD").Return(wallet, nil)
	mockPublisher.On("Publish", ctx, mock.AnythingOfType("events.BaseEvent")).Return(nil)

	// Execute
	err := service.ValidateBalance(ctx, userID, 10000, "USD")

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "insufficient balance")

	// Verify mocks
	mockRepo.AssertExpectations(t)
	mockPublisher.AssertExpectations(t)
}

func TestService_DeductBalance_Success(t *testing.T) {
	// Setup
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	mockRepo := &MockWalletRepository{}
	mockPublisher := &MockWalletEventPublisher{}

	service := NewService(mockRepo, mockPublisher, logger)

	userID := uuid.New()
	paymentID := uuid.New()
	walletID := uuid.New()
	wallet := &Wallet{
		ID:       walletID,
		UserID:   userID,
		Balance:  50000,
		Currency: "USD",
		Version:  1,
	}

	// Mock expectations
	mockRepo.On("GetWalletByUserID", ctx, userID, "USD").Return(wallet, nil)
	mockRepo.On("CreateTransaction", ctx, mock.AnythingOfType("*wallet.Transaction")).Return(nil)
	mockRepo.On("UpdateBalance", ctx, walletID, int64(40000), 1).Return(nil)
	mockPublisher.On("Publish", ctx, mock.AnythingOfType("events.WalletDeductedEvent")).Return(nil)

	// Execute
	err := service.DeductBalance(ctx, userID, 10000, "USD", paymentID)

	// Assert
	assert.NoError(t, err)

	// Verify mocks
	mockRepo.AssertExpectations(t)
	mockPublisher.AssertExpectations(t)
}

func TestService_DeductBalance_InsufficientFunds(t *testing.T) {
	// Setup
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	mockRepo := &MockWalletRepository{}
	mockPublisher := &MockWalletEventPublisher{}

	service := NewService(mockRepo, mockPublisher, logger)

	userID := uuid.New()
	paymentID := uuid.New()
	walletID := uuid.New()
	wallet := &Wallet{
		ID:       walletID,
		UserID:   userID,
		Balance:  5000,
		Currency: "USD",
		Version:  1,
	}

	// Mock expectations
	mockRepo.On("GetWalletByUserID", ctx, userID, "USD").Return(wallet, nil)

	// Execute
	err := service.DeductBalance(ctx, userID, 10000, "USD", paymentID)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "insufficient balance")

	// Verify mocks
	mockRepo.AssertExpectations(t)
}

func TestService_RefundBalance_Success(t *testing.T) {
	// Setup
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	mockRepo := &MockWalletRepository{}
	mockPublisher := &MockWalletEventPublisher{}

	service := NewService(mockRepo, mockPublisher, logger)

	userID := uuid.New()
	paymentID := uuid.New()
	walletID := uuid.New()
	wallet := &Wallet{
		ID:       walletID,
		UserID:   userID,
		Balance:  40000,
		Currency: "USD",
		Version:  2,
	}

	// Mock expectations
	mockRepo.On("GetWalletByUserID", ctx, userID, "USD").Return(wallet, nil)
	mockRepo.On("CreateTransaction", ctx, mock.AnythingOfType("*wallet.Transaction")).Return(nil)
	mockRepo.On("UpdateBalance", ctx, walletID, int64(50000), 2).Return(nil)
	mockPublisher.On("Publish", ctx, mock.AnythingOfType("events.WalletRefundedEvent")).Return(nil)

	// Execute
	err := service.RefundBalance(ctx, userID, 10000, "USD", paymentID, "payment_failed")

	// Assert
	assert.NoError(t, err)

	// Verify mocks
	mockRepo.AssertExpectations(t)
	mockPublisher.AssertExpectations(t)
}

func TestService_GetWallet_Success(t *testing.T) {
	// Setup
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	mockRepo := &MockWalletRepository{}
	mockPublisher := &MockWalletEventPublisher{}

	service := NewService(mockRepo, mockPublisher, logger)

	userID := uuid.New()
	walletID := uuid.New()
	expectedWallet := &Wallet{
		ID:       walletID,
		UserID:   userID,
		Balance:  50000,
		Currency: "USD",
		Version:  1,
	}

	// Mock expectations
	mockRepo.On("GetWalletByUserID", ctx, userID, "USD").Return(expectedWallet, nil)

	// Execute
	wallet, err := service.GetWallet(ctx, userID, "USD")

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, expectedWallet, wallet)

	// Verify mocks
	mockRepo.AssertExpectations(t)
}

func TestService_GetTransactionHistory_Success(t *testing.T) {
	// Setup
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	mockRepo := &MockWalletRepository{}
	mockPublisher := &MockWalletEventPublisher{}

	service := NewService(mockRepo, mockPublisher, logger)

	userID := uuid.New()
	walletID := uuid.New()
	wallet := &Wallet{
		ID:       walletID,
		UserID:   userID,
		Balance:  50000,
		Currency: "USD",
		Version:  1,
	}

	expectedTransactions := []*Transaction{
		{
			ID:       uuid.New(),
			WalletID: walletID,
			Type:     TypeDebit,
			Amount:   10000,
			Currency: "USD",
			Status:   TxnStatusCompleted,
		},
	}

	// Mock expectations
	mockRepo.On("GetWalletByUserID", ctx, userID, "USD").Return(wallet, nil)
	mockRepo.On("GetTransactionsByWallet", ctx, walletID).Return(expectedTransactions, nil)

	// Execute
	transactions, err := service.GetTransactionHistory(ctx, userID, "USD")

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, expectedTransactions, transactions)

	// Verify mocks
	mockRepo.AssertExpectations(t)
}

// Benchmark tests
func BenchmarkService_ValidateBalance(b *testing.B) {
	ctx := context.Background()
	logger := zaptest.NewLogger(b)

	mockRepo := &MockWalletRepository{}
	mockPublisher := &MockWalletEventPublisher{}

	service := NewService(mockRepo, mockPublisher, logger)

	userID := uuid.New()
	walletID := uuid.New()
	wallet := &Wallet{
		ID:       walletID,
		UserID:   userID,
		Balance:  50000,
		Currency: "USD",
		Version:  1,
	}

	mockRepo.On("GetWalletByUserID", ctx, userID, "USD").Return(wallet, nil)
	mockPublisher.On("Publish", ctx, mock.AnythingOfType("events.BaseEvent")).Return(nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		service.ValidateBalance(ctx, userID, 10000, "USD")
	}
}
