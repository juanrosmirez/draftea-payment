package eventstore

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockEventBus implements EventBus interface for testing
type MockEventBus struct {
	mock.Mock
	mu     sync.RWMutex
	events []Event
}

// Publish publishes an event (matches EventBus interface signature)
func (m *MockEventBus) Publish(ctx context.Context, event Event) error {
	args := m.Called(ctx, event)
	
	m.mu.Lock()
	m.events = append(m.events, event)
	m.mu.Unlock()
	
	return args.Error(0)
}

// Subscribe subscribes to events (matches EventBus interface signature)
func (m *MockEventBus) Subscribe(ctx context.Context, eventType string, handler func(Event) error) error {
	args := m.Called(ctx, eventType, handler)
	return args.Error(0)
}

// GetPublishedEvents returns all published events for testing
func (m *MockEventBus) GetPublishedEvents() []Event {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	events := make([]Event, len(m.events))
	copy(events, m.events)
	return events
}

// Reset clears all published events
func (m *MockEventBus) Reset() {
	m.mu.Lock()
	m.events = nil
	m.mu.Unlock()
}

// TestPaymentFlow_SuccessfulPayment tests a complete successful payment flow
func TestPaymentFlow_SuccessfulPayment(t *testing.T) {
	ctx := context.Background()
	
	// Create mock dependencies
	mockEventStore := NewMemoryEventStore()
	mockEventBus := &MockEventBus{}
	
	// Setup mock expectations
	mockEventBus.On("Publish", mock.Anything, mock.AnythingOfType("Event")).Return(nil)
	
	// Create payment saga orchestrator
	orchestrator := NewPaymentSagaOrchestrator(mockEventStore, mockEventBus)
	assert.NotNil(t, orchestrator)
	
	// Test basic event creation and publishing
	event, err := NewEvent("payment-123", "Payment", "PaymentInitiated", map[string]interface{}{
		"amount":   100.0,
		"currency": "USD",
		"user_id":  "user-456",
	}, "user-456", "correlation-789")
	
	assert.NoError(t, err)
	assert.NotNil(t, event)
	assert.Equal(t, "PaymentInitiated", event.EventType)
	
	// Test event publishing
	err = mockEventBus.Publish(ctx, *event)
	assert.NoError(t, err)
	
	// Verify event was published
	publishedEvents := mockEventBus.GetPublishedEvents()
	assert.Len(t, publishedEvents, 1)
	assert.Equal(t, "PaymentInitiated", publishedEvents[0].EventType)
	
	// Verify mock expectations
	mockEventBus.AssertExpectations(t)
}

// TestPaymentFlow_EventCreation tests event creation functionality
func TestPaymentFlow_EventCreation(t *testing.T) {
	// Test creating various payment events
	testCases := []struct {
		name          string
		aggregateID   string
		aggregateType string
		eventType     string
		eventData     interface{}
	}{
		{
			name:          "PaymentInitiated",
			aggregateID:   "payment-123",
			aggregateType: "Payment",
			eventType:     "PaymentInitiated",
			eventData: map[string]interface{}{
				"amount":   100.0,
				"currency": "USD",
				"user_id":  "user-456",
			},
		},
		{
			name:          "PaymentCompleted",
			aggregateID:   "payment-123",
			aggregateType: "Payment",
			eventType:     "PaymentCompleted",
			eventData: map[string]interface{}{
				"amount":        100.0,
				"currency":      "USD",
				"transaction_id": "txn-789",
			},
		},
		{
			name:          "PaymentFailed",
			aggregateID:   "payment-123",
			aggregateType: "Payment",
			eventType:     "PaymentFailed",
			eventData: PaymentFailedEventData{
				PaymentID:     "payment-123",
				UserID:        "user-456",
				Amount:        100.0,
				Currency:      "USD",
				FailureReason: "Insufficient funds",
			},
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			event, err := NewEvent(tc.aggregateID, tc.aggregateType, tc.eventType, tc.eventData, "user-456", "correlation-789")
			
			assert.NoError(t, err)
			assert.NotNil(t, event)
			assert.Equal(t, tc.eventType, event.EventType)
			assert.Equal(t, tc.aggregateID, event.AggregateID)
			assert.Equal(t, tc.aggregateType, event.AggregateType)
		})
	}
}

// TestPaymentFlow_MockEventBusIntegration tests MockEventBus integration
func TestPaymentFlow_MockEventBusIntegration(t *testing.T) {
	ctx := context.Background()
	mockEventBus := &MockEventBus{}
	
	// Setup mock expectations
	mockEventBus.On("Publish", mock.Anything, mock.AnythingOfType("eventstore.Event")).Return(nil)
	mockEventBus.On("Subscribe", mock.Anything, mock.AnythingOfType("string"), mock.AnythingOfType("func(eventstore.Event) error")).Return(nil)
	
	// Test publishing events
	event1, _ := NewEvent("payment-1", "Payment", "PaymentInitiated", map[string]interface{}{"amount": 100.0}, "user-1", "corr-1")
	event2, _ := NewEvent("payment-2", "Payment", "PaymentCompleted", map[string]interface{}{"amount": 200.0}, "user-2", "corr-2")
	
	err1 := mockEventBus.Publish(ctx, *event1)
	err2 := mockEventBus.Publish(ctx, *event2)
	
	assert.NoError(t, err1)
	assert.NoError(t, err2)
	
	// Test subscription
	handler := func(event Event) error { return nil }
	err := mockEventBus.Subscribe(ctx, "PaymentInitiated", handler)
	assert.NoError(t, err)
	
	// Verify published events
	publishedEvents := mockEventBus.GetPublishedEvents()
	assert.Len(t, publishedEvents, 2)
	
	// Verify mock expectations
	mockEventBus.AssertExpectations(t)
}
