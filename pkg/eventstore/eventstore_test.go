package eventstore

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemoryEventStore_BasicOperations(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryEventStore()
	defer store.Close()

	// Test AppendEvent
	aggregateID := uuid.New().String()
	event, err := NewEvent(aggregateID, "payment", "PaymentInitiated", 
		map[string]interface{}{"amount": 100.0}, "user123", "corr123")
	require.NoError(t, err)

	err = store.AppendEvent(ctx, event)
	require.NoError(t, err)
	assert.Equal(t, int64(1), event.SequenceNumber)
	assert.Equal(t, int64(1), event.AggregateVersion)

	// Test GetEventsByAggregate
	events, err := store.GetEventsByAggregate(ctx, aggregateID, 0)
	require.NoError(t, err)
	assert.Len(t, events, 1)
	assert.Equal(t, "PaymentInitiated", events[0].EventType)

	// Test GetLastSequenceNumber
	lastSeq, err := store.GetLastSequenceNumber(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), lastSeq)

	// Test GetAggregateVersion
	version, err := store.GetAggregateVersion(ctx, aggregateID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), version)
}

func TestMemoryEventStore_ConcurrencyControl(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryEventStore()
	defer store.Close()

	aggregateID := uuid.New().String()

	// Append first event
	event1, err := NewEvent(aggregateID, "payment", "PaymentInitiated", 
		map[string]interface{}{"amount": 100.0}, "user123", "corr123")
	require.NoError(t, err)
	
	err = store.AppendEvent(ctx, event1)
	require.NoError(t, err)

	// Try to append event with wrong version (should fail)
	event2, err := NewEvent(aggregateID, "payment", "PaymentCompleted", 
		map[string]interface{}{"gateway_tx": "tx123"}, "user123", "corr124")
	require.NoError(t, err)
	event2.AggregateVersion = 3 // Wrong version

	err = store.AppendEvent(ctx, event2)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "concurrency conflict")
}

func TestMemoryEventStore_EventFiltering(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryEventStore()
	defer store.Close()

	// Create events for different aggregates
	aggregateID1 := uuid.New().String()
	aggregateID2 := uuid.New().String()

	events := []*Event{
		mustCreateEvent(aggregateID1, "payment", "PaymentInitiated"),
		mustCreateEvent(aggregateID1, "payment", "PaymentCompleted"),
		mustCreateEvent(aggregateID2, "wallet", "WalletDeducted"),
	}

	for _, event := range events {
		err := store.AppendEvent(ctx, event)
		require.NoError(t, err)
	}

	// Filter by aggregate type
	filter := EventFilter{
		AggregateType: stringPtr("payment"),
	}
	
	filteredEvents, err := store.GetEvents(ctx, filter)
	require.NoError(t, err)
	assert.Len(t, filteredEvents, 2)

	// Filter by event types
	filter = EventFilter{
		EventTypes: []string{"PaymentInitiated"},
	}
	
	filteredEvents, err = store.GetEvents(ctx, filter)
	require.NoError(t, err)
	assert.Len(t, filteredEvents, 1)
	assert.Equal(t, "PaymentInitiated", filteredEvents[0].EventType)
}

func TestAggregateRepository_SaveAndLoad(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryEventStore()
	defer store.Close()

	repo := NewAggregateRepository(store, 5) // Snapshot every 5 events

	// Create and save payment aggregate
	paymentID := uuid.New().String()
	userID := uuid.New().String()
	
	payment := NewPaymentAggregate(paymentID, userID, 100.50, "USD", "credit_card")
	
	err := payment.InitiatePayment()
	require.NoError(t, err)

	err = repo.Save(ctx, payment, userID, uuid.New().String())
	require.NoError(t, err)

	// Load the aggregate
	loadedAggregate, err := repo.Load(ctx, paymentID, func() Aggregate {
		return &PaymentAggregate{}
	})
	require.NoError(t, err)

	loadedPayment := loadedAggregate.(*PaymentAggregate)
	assert.Equal(t, paymentID, loadedPayment.GetID())
	assert.Equal(t, userID, loadedPayment.UserID)
	assert.Equal(t, 100.50, loadedPayment.Amount)
	assert.Equal(t, "USD", loadedPayment.Currency)
	assert.Equal(t, "initiated", loadedPayment.Status)
	assert.Equal(t, int64(1), loadedPayment.GetVersion())
}

func TestPaymentAggregate_BusinessLogic(t *testing.T) {
	paymentID := uuid.New().String()
	userID := uuid.New().String()
	
	payment := NewPaymentAggregate(paymentID, userID, 100.50, "USD", "credit_card")

	// Test initiate payment
	err := payment.InitiatePayment()
	require.NoError(t, err)
	assert.Len(t, payment.GetUncommittedEvents(), 1)

	// Test complete payment (should fail - wrong status)
	err = payment.CompletePayment("gateway_tx_123")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot complete payment with status")

	// Simulate processing status
	payment.Status = "processing"
	
	// Now complete payment should work
	err = payment.CompletePayment("gateway_tx_123")
	require.NoError(t, err)
	assert.Len(t, payment.GetUncommittedEvents(), 2)

	// Test fail completed payment (should fail)
	err = payment.FailPayment("test failure")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot fail completed payment")
}

func TestWalletAggregate_BusinessLogic(t *testing.T) {
	walletID := uuid.New().String()
	userID := uuid.New().String()
	
	wallet := NewWalletAggregate(walletID, userID, "USD")

	// Test add balance
	err := wallet.AddBalance(1000.0, "Initial deposit")
	require.NoError(t, err)
	assert.Len(t, wallet.GetUncommittedEvents(), 1)

	// Apply the event to update state
	events := wallet.GetUncommittedEvents()
	err = wallet.ApplyEvent(events[0])
	require.NoError(t, err)
	assert.Equal(t, 1000.0, wallet.Balance)

	// Test deduct balance
	err = wallet.DeductBalance(300.0, "Purchase")
	require.NoError(t, err)
	
	// Apply the event
	events = wallet.GetUncommittedEvents()
	err = wallet.ApplyEvent(events[1]) // Second event
	require.NoError(t, err)
	assert.Equal(t, 700.0, wallet.Balance)

	// Test insufficient balance
	err = wallet.DeductBalance(800.0, "Large purchase")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "insufficient balance")
}

func TestEventStore_Snapshots(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryEventStore()
	defer store.Close()

	aggregateID := uuid.New().String()
	snapshotData := json.RawMessage(`{"id":"test","balance":500.0}`)

	// Create snapshot
	err := store.CreateSnapshot(ctx, aggregateID, "wallet", 5, snapshotData)
	require.NoError(t, err)

	// Get snapshot
	snapshot, err := store.GetSnapshot(ctx, aggregateID)
	require.NoError(t, err)
	require.NotNil(t, snapshot)
	
	assert.Equal(t, aggregateID, snapshot.AggregateID)
	assert.Equal(t, "wallet", snapshot.AggregateType)
	assert.Equal(t, int64(5), snapshot.AggregateVersion)
	assert.Equal(t, snapshotData, snapshot.Data)
}

func TestEventStore_Subscription(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	store := NewMemoryEventStore()
	defer store.Close()

	// Subscribe to events
	eventChan, err := store.Subscribe(ctx, EventFilter{})
	require.NoError(t, err)

	// Create and append event in goroutine
	go func() {
		time.Sleep(100 * time.Millisecond)
		event, _ := NewEvent(uuid.New().String(), "payment", "PaymentInitiated", 
			map[string]interface{}{"amount": 100.0}, "user123", "corr123")
		store.AppendEvent(ctx, event)
	}()

	// Wait for event
	select {
	case receivedEvent := <-eventChan:
		assert.Equal(t, "PaymentInitiated", receivedEvent.EventType)
	case <-ctx.Done():
		t.Fatal("Timeout waiting for event")
	}
}

// Helper functions
func mustCreateEvent(aggregateID, aggregateType, eventType string) *Event {
	event, err := NewEvent(aggregateID, aggregateType, eventType, 
		map[string]interface{}{"test": "data"}, "user123", "corr123")
	if err != nil {
		panic(err)
	}
	return event
}

func stringPtr(s string) *string {
	return &s
}

// Benchmark tests
func BenchmarkMemoryEventStore_AppendEvent(b *testing.B) {
	ctx := context.Background()
	store := NewMemoryEventStore()
	defer store.Close()

	aggregateID := uuid.New().String()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		event, _ := NewEvent(aggregateID, "payment", "PaymentInitiated", 
			map[string]interface{}{"amount": float64(i)}, "user123", "corr123")
		store.AppendEvent(ctx, event)
	}
}

func BenchmarkMemoryEventStore_GetEvents(b *testing.B) {
	ctx := context.Background()
	store := NewMemoryEventStore()
	defer store.Close()

	// Prepare test data
	aggregateID := uuid.New().String()
	for i := 0; i < 1000; i++ {
		event, _ := NewEvent(aggregateID, "payment", "PaymentInitiated", 
			map[string]interface{}{"amount": float64(i)}, "user123", "corr123")
		store.AppendEvent(ctx, event)
	}

	filter := EventFilter{AggregateID: &aggregateID}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.GetEvents(ctx, filter)
	}
}
