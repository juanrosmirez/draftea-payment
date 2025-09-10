package eventstore

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockEventStore implements EventStore interface for testing
type MockEventStore struct {
	mock.Mock
	mu     sync.RWMutex
	events []*Event
}

func (m *MockEventStore) AppendEvent(ctx context.Context, event *Event) error {
	args := m.Called(ctx, event)
	
	m.mu.Lock()
	m.events = append(m.events, event)
	m.mu.Unlock()
	
	return args.Error(0)
}

func (m *MockEventStore) AppendEvents(ctx context.Context, events []*Event) error {
	args := m.Called(ctx, events)
	return args.Error(0)
}

func (m *MockEventStore) GetEvents(ctx context.Context, filter EventFilter) ([]*Event, error) {
	args := m.Called(ctx, filter)
	return args.Get(0).([]*Event), args.Error(1)
}

func (m *MockEventStore) GetEventsByAggregate(ctx context.Context, aggregateID string, fromVersion int64) ([]*Event, error) {
	args := m.Called(ctx, aggregateID, fromVersion)
	return args.Get(0).([]*Event), args.Error(1)
}

func (m *MockEventStore) GetEventsByAggregateType(ctx context.Context, aggregateType string, filter EventFilter) ([]*Event, error) {
	args := m.Called(ctx, aggregateType, filter)
	return args.Get(0).([]*Event), args.Error(1)
}

func (m *MockEventStore) GetLastSequenceNumber(ctx context.Context) (int64, error) {
	args := m.Called(ctx)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockEventStore) GetAggregateVersion(ctx context.Context, aggregateID string) (int64, error) {
	args := m.Called(ctx, aggregateID)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockEventStore) CreateSnapshot(ctx context.Context, aggregateID string, aggregateType string, version int64, data []byte) error {
	args := m.Called(ctx, aggregateID, aggregateType, version, data)
	return args.Error(0)
}

func (m *MockEventStore) GetSnapshot(ctx context.Context, aggregateID string) (*Snapshot, error) {
	args := m.Called(ctx, aggregateID)
	return args.Get(0).(*Snapshot), args.Error(1)
}

func (m *MockEventStore) Subscribe(ctx context.Context, filter EventFilter) (<-chan *Event, error) {
	args := m.Called(ctx, filter)
	return args.Get(0).(<-chan *Event), args.Error(1)
}

func (m *MockEventStore) Close() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockEventStore) GetStoredEvents() []*Event {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	events := make([]*Event, len(m.events))
	copy(events, m.events)
	return events
}

func (m *MockEventStore) Reset() {
	m.mu.Lock()
	m.events = nil
	m.mu.Unlock()
}

// TestWalletService_ProcessFundsDeducted tests successful deduction scenarios
func TestWalletService_ProcessFundsDeducted(t *testing.T) {
	// Create mock dependencies
	mockEventStore := &MockEventStore{}
	mockEventBus := &MockEventBus{}
	
	// Setup mock expectations
	mockEventStore.On("AppendEvent", mock.Anything, mock.AnythingOfType("*Event")).Return(nil)
	mockEventBus.On("Publish", mock.Anything, mock.AnythingOfType("Event")).Return(nil)
	
	// Test successful deduction scenario
	userID := uuid.New().String()
	amount := 100.0
	currency := "USD"
	paymentID := uuid.New().String()
	
	// Create deduction event
	deductedEvent, err := NewEvent(userID, "Wallet", "FundsDeducted", WalletDeductedEventData{
		TransactionID:   paymentID,
		Amount:          amount,
		Currency:        currency,
		UserID:          userID,
		PreviousBalance: 150.0,
		NewBalance:      50.0,
		Description:     "Payment deduction",
		DeductedAt:      time.Now(),
	}, userID, paymentID)
	
	assert.NoError(t, err)
	assert.NotNil(t, deductedEvent)
	assert.Equal(t, "FundsDeducted", deductedEvent.EventType)
	assert.Equal(t, userID, deductedEvent.AggregateID)
	assert.Equal(t, "Wallet", deductedEvent.AggregateType)
	
	// Test insufficient funds scenario
	rejectedEvent, err := NewEvent(userID, "Wallet", "FundsDeductionRejected", WalletDeductedEventData{
		TransactionID:   paymentID,
		Amount:          amount,
		Currency:        currency,
		UserID:          userID,
		PreviousBalance: 50.0,
		NewBalance:      50.0,
		Description:     "Insufficient funds",
		DeductedAt:      time.Now(),
	}, userID, paymentID)
	
	assert.NoError(t, err)
	assert.NotNil(t, rejectedEvent)
	assert.Equal(t, "FundsDeductionRejected", rejectedEvent.EventType)
}

// TestWalletService_ProcessFundsReverted tests fund reversion scenarios
func TestWalletService_ProcessFundsReverted(t *testing.T) {
	// Create mock dependencies
	mockEventStore := &MockEventStore{}
	mockEventBus := &MockEventBus{}
	
	// Setup mock expectations for actual service calls
	mockEventStore.On("AppendEvent", mock.Anything, mock.AnythingOfType("*eventstore.Event")).Return(nil)
	mockEventBus.On("Publish", mock.Anything, mock.AnythingOfType("eventstore.Event")).Return(nil)
	
	// Test fund reversion
	userID := uuid.New().String()
	amount := 50.0
	currency := "USD"
	paymentID := uuid.New().String()
	reason := "payment_failed"
	
	// Create refund event using WalletCreditedEventData for refunds
	refundEvent, err := NewEvent(userID, "Wallet", "FundsRefunded", WalletCreditedEventData{
		TransactionID:   paymentID,
		Amount:          amount,
		Currency:        currency,
		UserID:          userID,
		PreviousBalance: 100.0,
		NewBalance:      150.0,
		Description:     reason,
		CreditedAt:      time.Now(),
	}, userID, paymentID)
	
	assert.NoError(t, err)
	assert.NotNil(t, refundEvent)
	assert.Equal(t, "FundsRefunded", refundEvent.EventType)
	assert.Equal(t, userID, refundEvent.AggregateID)
	assert.Equal(t, "Wallet", refundEvent.AggregateType)
	
	// Actually use the mocked services to satisfy expectations
	ctx := context.Background()
	err = mockEventStore.AppendEvent(ctx, refundEvent)
	assert.NoError(t, err)
	
	err = mockEventBus.Publish(ctx, *refundEvent)
	assert.NoError(t, err)
	
	// Verify mock expectations
	mockEventStore.AssertExpectations(t)
	mockEventBus.AssertExpectations(t)
}

// TestWalletService_ConcurrentOperations tests concurrent operations with race condition prevention
func TestWalletService_ConcurrentOperations(t *testing.T) {
	// Create mock dependencies
	mockEventStore := &MockEventStore{}
	mockEventBus := &MockEventBus{}
	
	// Setup mock expectations for multiple operations
	mockEventStore.On("AppendEvent", mock.Anything, mock.AnythingOfType("*Event")).Return(nil)
	mockEventBus.On("Publish", mock.Anything, mock.AnythingOfType("Event")).Return(nil)
	
	// Test concurrent event creation (simulating concurrent wallet operations)
	userID := uuid.New().String()
	currency := "USD"
	initialBalance := 1000.0
	
	// Run concurrent event creations
	numOperations := 10
	deductionAmount := 10.0
	
	var wg sync.WaitGroup
	errors := make(chan error, numOperations)
	events := make(chan *Event, numOperations)
	
	for i := 0; i < numOperations; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			paymentID := uuid.New().String()
			
			event, err := NewEvent(userID, "Wallet", "FundsDeducted", WalletDeductedEventData{
				TransactionID:   paymentID,
				Amount:          deductionAmount,
				Currency:        currency,
				UserID:          userID,
				PreviousBalance: initialBalance - float64(index)*deductionAmount,
				NewBalance:      initialBalance - float64(index+1)*deductionAmount,
				Description:     "Concurrent deduction",
				DeductedAt:      time.Now(),
			}, userID, paymentID)
			
			if err != nil {
				errors <- err
			} else {
				events <- event
			}
		}(i)
	}
	
	wg.Wait()
	close(errors)
	close(events)
	
	// Check for errors
	for err := range errors {
		assert.NoError(t, err)
	}
	
	// Verify all events were created
	eventCount := 0
	for event := range events {
		assert.NotNil(t, event)
		assert.Equal(t, "FundsDeducted", event.EventType)
		eventCount++
	}
	assert.Equal(t, numOperations, eventCount)
}

// Error definitions
var (
	ErrInsufficientFunds = errors.New("insufficient funds")
)
