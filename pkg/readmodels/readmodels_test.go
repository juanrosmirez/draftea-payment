package readmodels

import (
	"context"
	"log"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/google/uuid"

	"payment-system/pkg/eventstore"
)

func TestMemoryReadModelStore_BasicOperations(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryReadModelStore()
	defer store.Close()

	// Test Set and Get
	testData := map[string]interface{}{
		"user_id": "user123",
		"balance": 100.50,
	}

	err := store.Set(ctx, "test_key", testData)
	require.NoError(t, err)

	var retrieved map[string]interface{}
	err = store.Get(ctx, "test_key", &retrieved)
	require.NoError(t, err)
	assert.Equal(t, "user123", retrieved["user_id"])
	assert.Equal(t, 100.50, retrieved["balance"])

	// Test Delete
	err = store.Delete(ctx, "test_key")
	require.NoError(t, err)

	err = store.Get(ctx, "test_key", &retrieved)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "key not found")
}

func TestBalanceProjectionService_WalletEvents(t *testing.T) {
	ctx := context.Background()
	
	eventStore := eventstore.NewMemoryEventStore()
	defer eventStore.Close()
	
	readModelStore := NewMemoryReadModelStore()
	defer readModelStore.Close()
	
	logger := log.New(os.Stdout, "[TEST] ", log.LstdFlags)
	service := NewBalanceProjectionService(readModelStore, eventStore, logger)
	
	err := service.Start(ctx)
	require.NoError(t, err)
	defer service.Stop()
	
	// Wait for service to initialize
	time.Sleep(100 * time.Millisecond)
	
	userID := uuid.New().String()
	walletID := uuid.New().String()
	currency := "USD"
	
	// Test initial balance (should be zero)
	balance, err := service.GetBalance(ctx, userID, currency)
	require.NoError(t, err)
	assert.Equal(t, 0.0, balance.Balance)
	assert.Equal(t, userID, balance.UserID)
	assert.Equal(t, currency, balance.Currency)
	
	// Add debit event
	debitEvent, err := eventstore.NewEvent(walletID, "wallet", "WalletCredited", eventstore.WalletCreditedEventData{
		TransactionID:   uuid.New().String(),
		Amount:          500.0,
		PreviousBalance: 0.0,
		NewBalance:      500.0,
		Description:     "Initial deposit",
		CreditedAt:      time.Now().UTC(),
		UserID:          userID,
		Currency:        currency,
	}, userID, uuid.New().String())
	require.NoError(t, err)
	
	err = eventStore.AppendEvent(ctx, debitEvent)
	require.NoError(t, err)
	
	// Wait for processing
	time.Sleep(200 * time.Millisecond)
	
	// Check updated balance
	balance, err = service.GetBalance(ctx, userID, currency)
	require.NoError(t, err)
	assert.Equal(t, 500.0, balance.Balance)
	assert.Equal(t, int64(1), balance.TransactionCount)
	
	// Add debit event
	debitEvent, err = eventstore.NewEvent(walletID, "wallet", "WalletDeducted", eventstore.WalletDeductedEventData{
		TransactionID:   uuid.New().String(),
		Amount:          150.0,
		PreviousBalance: 500.0,
		NewBalance:      350.0,
		Description:     "Purchase",
		DeductedAt:      time.Now().UTC(),
		UserID:          userID,
		Currency:        currency,
	}, userID, uuid.New().String())
	require.NoError(t, err)
	
	err = eventStore.AppendEvent(ctx, debitEvent)
	require.NoError(t, err)
	
	time.Sleep(200 * time.Millisecond)
	
	// Check final balance
	balance, err = service.GetBalance(ctx, userID, currency)
	require.NoError(t, err)
	assert.Equal(t, 350.0, balance.Balance)
	assert.Equal(t, int64(2), balance.TransactionCount)
}

func TestBalanceProjectionService_WalletSummary(t *testing.T) {
	ctx := context.Background()
	
	eventStore := eventstore.NewMemoryEventStore()
	defer eventStore.Close()
	
	readModelStore := NewMemoryReadModelStore()
	defer readModelStore.Close()
	
	logger := log.New(os.Stdout, "[TEST] ", log.LstdFlags)
	service := NewBalanceProjectionService(readModelStore, eventStore, logger)
	
	err := service.Start(ctx)
	require.NoError(t, err)
	defer service.Stop()
	
	time.Sleep(100 * time.Millisecond)
	
	userID := uuid.New().String()
	walletID := uuid.New().String()
	currency := "USD"
	
	// Multiple transactions
	transactions := []struct {
		eventType string
		amount    float64
		balance   float64
	}{
		{"WalletCredited", 1000.0, 1000.0},
		{"WalletDeducted", 200.0, 800.0},
		{"WalletDeducted", 150.0, 650.0},
		{"WalletCredited", 300.0, 950.0},
		{"WalletRefunded", 50.0, 1000.0},
	}
	
	for _, tx := range transactions {
		var event *eventstore.Event
		var err error
		
		switch tx.eventType {
		case "WalletCredited":
			event, err = eventstore.NewEvent(walletID, "wallet", "WalletCredited", eventstore.WalletCreditedEventData{
				TransactionID:   uuid.New().String(),
				Amount:          tx.amount,
				PreviousBalance: tx.balance - tx.amount,
				NewBalance:      tx.balance,
				Description:     "Transaction",
				CreditedAt:      time.Now().UTC(),
				UserID:          userID,
				Currency:        currency,
			}, userID, uuid.New().String())
		case "WalletDeducted":
			event, err = eventstore.NewEvent(walletID, "wallet", "WalletDeducted", eventstore.WalletDeductedEventData{
				TransactionID:   uuid.New().String(),
				Amount:          tx.amount,
				PreviousBalance: tx.balance + tx.amount,
				NewBalance:      tx.balance,
				Description:     "Transaction",
				DeductedAt:      time.Now().UTC(),
				UserID:          userID,
				Currency:        currency,
			}, userID, uuid.New().String())
		case "WalletRefunded":
			event, err = eventstore.NewEvent(walletID, "wallet", "WalletRefunded", eventstore.WalletCreditedEventData{
				TransactionID:   uuid.New().String(),
				Amount:          tx.amount,
				PreviousBalance: tx.balance - tx.amount,
				NewBalance:      tx.balance,
				Description:     "Test refund",
				CreditedAt:      time.Now().UTC(),
				UserID:          userID,
				Currency:        currency,
			}, userID, uuid.New().String())
		}
		
		require.NoError(t, err)
		err = eventStore.AppendEvent(ctx, event)
		require.NoError(t, err)
	}
	
	// Wait for all events to be processed
	time.Sleep(500 * time.Millisecond)
	
	// Check wallet summary
	summary, err := service.GetWalletSummary(ctx, userID, currency)
	require.NoError(t, err)
	
	assert.Equal(t, userID, summary.UserID)
	assert.Equal(t, currency, summary.Currency)
	assert.Equal(t, 1000.0, summary.CurrentBalance)
	assert.Equal(t, 1350.0, summary.TotalCredits)  // 1000 + 300 + 50
	assert.Equal(t, 350.0, summary.TotalDebits)    // 200 + 150
	assert.Equal(t, int64(5), summary.TransactionCount)
}

func TestBalanceProjectionService_PaymentEvents(t *testing.T) {
	ctx := context.Background()
	
	eventStore := eventstore.NewMemoryEventStore()
	defer eventStore.Close()
	
	readModelStore := NewMemoryReadModelStore()
	defer readModelStore.Close()
	
	logger := log.New(os.Stdout, "[TEST] ", log.LstdFlags)
	service := NewBalanceProjectionService(readModelStore, eventStore, logger)
	
	err := service.Start(ctx)
	require.NoError(t, err)
	defer service.Stop()
	
	time.Sleep(100 * time.Millisecond)
	
	userID := uuid.New().String()
	
	// Payment events
	paymentEvents := []struct {
		eventType string
		amount    float64
	}{
		{"PaymentInitiated", 100.0},
		{"PaymentInitiated", 200.0},
		{"PaymentCompleted", 0.0}, // Amount not relevant for completion
		{"PaymentFailed", 0.0},
	}
	
	for _, pe := range paymentEvents {
		paymentID := uuid.New().String()
		var event *eventstore.Event
		var err error
		
		switch pe.eventType {
		case "PaymentInitiated":
			event, err = eventstore.NewEvent(paymentID, "payment", "PaymentInitiated", eventstore.PaymentInitiatedEventData{
				UserID:        userID,
				Amount:        pe.amount,
				Currency:      "USD",
				PaymentMethod: "wallet",
				InitiatedAt:   time.Now().UTC(),
			}, userID, uuid.New().String())
		case "PaymentCompleted":
			event, err = eventstore.NewEvent(paymentID, "payment", "PaymentCompleted", eventstore.PaymentCompletedEventData{
				GatewayTransactionID: "gw_tx_123",
				CompletedAt:          time.Now().UTC(),
				ProcessingTimeMs:     1000,
			}, userID, uuid.New().String())
		case "PaymentFailed":
			event, err = eventstore.NewEvent(paymentID, "payment", "PaymentFailed", eventstore.PaymentFailedEventData{
				FailureReason: "Insufficient funds",
				FailedAt:      time.Now().UTC(),
			}, userID, uuid.New().String())
		}
		
		require.NoError(t, err)
		err = eventStore.AppendEvent(ctx, event)
		require.NoError(t, err)
		
		// Small delay between events
		time.Sleep(50 * time.Millisecond)
	}
	
	// Wait for processing
	time.Sleep(300 * time.Millisecond)
	
	// Check payment status
	status, err := service.GetPaymentStatus(ctx, userID)
	require.NoError(t, err)
	
	assert.Equal(t, userID, status.UserID)
	assert.Equal(t, int64(2), status.TotalPayments)     // 2 initiated
	assert.Equal(t, int64(1), status.CompletedPayments) // 1 completed
	assert.Equal(t, int64(1), status.FailedPayments)    // 1 failed
	assert.Equal(t, int64(0), status.PendingPayments)   // 0 pending (both resolved)
	assert.Equal(t, 300.0, status.TotalAmount)          // 100 + 200
}

func TestReadModelProjector_CatchUp(t *testing.T) {
	ctx := context.Background()
	
	eventStore := eventstore.NewMemoryEventStore()
	defer eventStore.Close()
	
	// Create events BEFORE starting projector
	userID := uuid.New().String()
	walletID := uuid.New().String()
	
	historicalEvents := []struct {
		eventType string
		amount    float64
		balance   float64
	}{
		{"WalletCredited", 500.0, 500.0},
		{"WalletDeducted", 100.0, 400.0},
		{"WalletCredited", 200.0, 600.0},
	}
	
	for _, he := range historicalEvents {
		var event *eventstore.Event
		var err error
		
		if he.eventType == "WalletCredited" {
			event, err = eventstore.NewEvent(walletID, "wallet", "WalletCredited", eventstore.WalletCreditedEventData{
				TransactionID:   uuid.New().String(),
				Amount:          he.amount,
				PreviousBalance: he.balance - he.amount,
				NewBalance:      he.balance,
				Description:     "Historical transaction",
				CreditedAt:      time.Now().UTC(),
				UserID:          userID,
				Currency:        "USD",
			}, userID, uuid.New().String())
		} else {
			event, err = eventstore.NewEvent(walletID, "wallet", "WalletDeducted", eventstore.WalletDeductedEventData{
				TransactionID:   uuid.New().String(),
				Amount:          he.amount,
				PreviousBalance: he.balance + he.amount,
				NewBalance:      he.balance,
				Description:     "Historical transaction",
				DeductedAt:      time.Now().UTC(),
				UserID:          userID,
				Currency:        "USD",
			}, userID, uuid.New().String())
		}
		
		require.NoError(t, err)
		err = eventStore.AppendEvent(ctx, event)
		require.NoError(t, err)
	}
	
	// NOW start the projector (should catch up)
	readModelStore := NewMemoryReadModelStore()
	defer readModelStore.Close()
	
	logger := log.New(os.Stdout, "[TEST] ", log.LstdFlags)
	service := NewBalanceProjectionService(readModelStore, eventStore, logger)
	
	err := service.Start(ctx)
	require.NoError(t, err)
	defer service.Stop()
	
	// Wait for catch-up
	time.Sleep(300 * time.Millisecond)
	
	// Verify catch-up worked
	balance, err := service.GetBalance(ctx, userID, "USD")
	require.NoError(t, err)
	assert.Equal(t, 600.0, balance.Balance)
	assert.Equal(t, int64(3), balance.TransactionCount)
}

func TestReadModelProjector_RealTimeEvents(t *testing.T) {
	ctx := context.Background()
	
	eventStore := eventstore.NewMemoryEventStore()
	defer eventStore.Close()
	
	readModelStore := NewMemoryReadModelStore()
	defer readModelStore.Close()
	
	logger := log.New(os.Stdout, "[TEST] ", log.LstdFlags)
	service := NewBalanceProjectionService(readModelStore, eventStore, logger)
	
	err := service.Start(ctx)
	require.NoError(t, err)
	defer service.Stop()
	
	time.Sleep(100 * time.Millisecond)
	
	userID := uuid.New().String()
	walletID := uuid.New().String()
	
	// Add real-time events
	debitEvent, err := eventstore.NewEvent(walletID, "wallet", "WalletCredited", eventstore.WalletCreditedEventData{
		TransactionID:   uuid.New().String(),
		Amount:          300.0,
		PreviousBalance: 0.0,
		NewBalance:      300.0,
		Description:     "Real-time deposit",
		CreditedAt:      time.Now().UTC(),
		UserID:          userID,
		Currency:        "USD",
	}, userID, uuid.New().String())
	require.NoError(t, err)
	
	err = eventStore.AppendEvent(ctx, debitEvent)
	require.NoError(t, err)
	
	// Should be processed immediately
	time.Sleep(200 * time.Millisecond)
	
	balance, err := service.GetBalance(ctx, userID, "USD")
	require.NoError(t, err)
	assert.Equal(t, 300.0, balance.Balance)
	assert.Equal(t, int64(1), balance.TransactionCount)
}

func TestBalanceProjectionService_GetAllBalances(t *testing.T) {
	ctx := context.Background()
	
	eventStore := eventstore.NewMemoryEventStore()
	defer eventStore.Close()
	
	readModelStore := NewMemoryReadModelStore()
	defer readModelStore.Close()
	
	logger := log.New(os.Stdout, "[TEST] ", log.LstdFlags)
	service := NewBalanceProjectionService(readModelStore, eventStore, logger)
	
	// Create multiple users with balances first, then start the service
	users := []struct {
		userID  string
		balance float64
	}{
		{uuid.New().String(), 100.0},
		{uuid.New().String(), 200.0},
		{uuid.New().String(), 300.0},
	}
	
	for _, user := range users {
		walletID := uuid.New().String()
		
		debitEvent, err := eventstore.NewEvent(walletID, "wallet", "WalletCredited", eventstore.WalletCreditedEventData{
			TransactionID:   uuid.New().String(),
			Amount:          user.balance,
			PreviousBalance: 0.0,
			NewBalance:      user.balance,
			Description:     "Initial deposit",
			CreditedAt:      time.Now().UTC(),
			UserID:          user.userID,
			Currency:        "USD",
		}, user.userID, uuid.New().String())
		require.NoError(t, err)
		
		err = eventStore.AppendEvent(ctx, debitEvent)
		require.NoError(t, err)
	}
	
	// Now start the service to process existing events via catchUp
	err := service.Start(ctx)
	require.NoError(t, err)
	defer service.Stop()
	
	// Wait for processing to complete
	time.Sleep(3 * time.Second)
	
	// Debug: Check what's in the store
	allData, err := readModelStore.GetAll(ctx, "balance:*")
	require.NoError(t, err)
	t.Logf("Raw store data: %+v", allData)
	
	// Debug: Check events in event store
	allEvents, err := eventStore.GetEvents(ctx, eventstore.EventFilter{})
	require.NoError(t, err)
	t.Logf("Events in store: %d", len(allEvents))
	for i, event := range allEvents {
		t.Logf("Event %d: %s (Type: %s, Seq: %d)", i+1, event.ID, event.EventType, event.SequenceNumber)
	}
	
	// Get all balances
	allBalances, err := service.GetAllBalances(ctx)
	require.NoError(t, err)
	
	t.Logf("Processed balances: %+v", allBalances)
	assert.Len(t, allBalances, 3)
	
	// Verify each balance
	for _, user := range users {
		found := false
		for _, balance := range allBalances {
			if balance.UserID == user.userID {
				assert.Equal(t, user.balance, balance.Balance)
				found = true
				break
			}
		}
		assert.True(t, found, "Balance not found for user %s", user.userID)
	}
}

// Benchmark tests
func BenchmarkBalanceProjectionService_ProcessEvent(b *testing.B) {
	ctx := context.Background()
	
	eventStore := eventstore.NewMemoryEventStore()
	defer eventStore.Close()
	
	readModelStore := NewMemoryReadModelStore()
	defer readModelStore.Close()
	
	logger := log.New(os.Stdout, "[TEST] ", log.LstdFlags)
	service := NewBalanceProjectionService(readModelStore, eventStore, logger)
	service.Start(ctx)
	defer service.Stop()
	
	time.Sleep(100 * time.Millisecond)
	
	userID := uuid.New().String()
	walletID := uuid.New().String()
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		creditEvent, _ := eventstore.NewEvent(walletID, "wallet", "WalletCredited", eventstore.WalletCreditedEventData{
			TransactionID:   uuid.New().String(),
			Amount:          float64(i),
			PreviousBalance: float64(i - 1),
			NewBalance:      float64(i),
			Description:     "Benchmark transaction",
			CreditedAt:      time.Now().UTC(),
			UserID:          userID,
			Currency:        "USD",
		}, userID, uuid.New().String())
		
		eventStore.AppendEvent(ctx, creditEvent)
	}
}

func BenchmarkBalanceProjectionService_GetBalance(b *testing.B) {
	ctx := context.Background()
	
	eventStore := eventstore.NewMemoryEventStore()
	defer eventStore.Close()
	
	readModelStore := NewMemoryReadModelStore()
	defer readModelStore.Close()
	
	logger := log.New(os.Stdout, "[TEST] ", log.LstdFlags)
	service := NewBalanceProjectionService(readModelStore, eventStore, logger)
	service.Start(ctx)
	defer service.Stop()
	
	time.Sleep(100 * time.Millisecond)
	
	userID := uuid.New().String()
	walletID := uuid.New().String()
	
	// Create initial balance
	creditEvent, _ := eventstore.NewEvent(walletID, "wallet", "WalletCredited", eventstore.WalletCreditedEventData{
		TransactionID:   uuid.New().String(),
		Amount:          1000.0,
		PreviousBalance: 0.0,
		NewBalance:      1000.0,
		Description:     "Initial deposit",
		CreditedAt:      time.Now().UTC(),
		UserID:          userID,
		Currency:        "USD",
	}, userID, uuid.New().String())
	
	eventStore.AppendEvent(ctx, creditEvent)
	time.Sleep(200 * time.Millisecond)
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		service.GetBalance(ctx, userID, "USD")
	}
}
