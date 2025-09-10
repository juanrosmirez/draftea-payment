package eventstore

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// PaymentAggregate representa un agregado de pago
type PaymentAggregate struct {
	BaseAggregate
	UserID        string  `json:"user_id"`
	Amount        float64 `json:"amount"`
	Currency      string  `json:"currency"`
	PaymentMethod string  `json:"payment_method"`
	Status        string  `json:"status"`
	GatewayTxID   string  `json:"gateway_tx_id,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
}

// NewPaymentAggregate crea un nuevo agregado de pago
func NewPaymentAggregate(id, userID string, amount float64, currency, paymentMethod string) *PaymentAggregate {
	return &PaymentAggregate{
		BaseAggregate: BaseAggregate{
			ID:      id,
			Type:    "payment",
			Version: 0,
			UncommittedEvents: make([]*Event, 0),
		},
		UserID:        userID,
		Amount:        amount,
		Currency:      currency,
		PaymentMethod: paymentMethod,
		Status:        "pending",
		CreatedAt:     time.Now().UTC(),
	}
}

// InitiatePayment inicia el proceso de pago
func (p *PaymentAggregate) InitiatePayment() error {
	if p.Status != "pending" {
		return fmt.Errorf("cannot initiate payment with status: %s", p.Status)
	}

	eventData := PaymentInitiatedEventData{
		PaymentID:     p.ID,
		UserID:        p.UserID,
		Amount:        p.Amount,
		Currency:      p.Currency,
		PaymentMethod: p.PaymentMethod,
		InitiatedAt:   time.Now().UTC(),
	}

	event, err := NewEvent(p.ID, "payment", "PaymentInitiated", eventData, p.UserID, uuid.New().String())
	if err != nil {
		return fmt.Errorf("failed to create PaymentInitiated event: %w", err)
	}

	p.AddUncommittedEvent(event)
	return p.ApplyEvent(event)
}

// CompletePayment completa el pago
func (p *PaymentAggregate) CompletePayment(gatewayTxID string) error {
	if p.Status != "processing" {
		return fmt.Errorf("cannot complete payment with status: %s", p.Status)
	}

	now := time.Now().UTC()
	eventData := PaymentCompletedEventData{
		PaymentID:   p.ID,
		UserID:      p.UserID,
		Amount:      p.Amount,
		Currency:    p.Currency,
		GatewayTxID: gatewayTxID,
		CompletedAt: now,
	}

	event, err := NewEvent(p.ID, "payment", "PaymentCompleted", eventData, p.UserID, uuid.New().String())
	if err != nil {
		return fmt.Errorf("failed to create PaymentCompleted event: %w", err)
	}

	p.AddUncommittedEvent(event)
	return p.ApplyEvent(event)
}

// FailPayment marca el pago como fallido
func (p *PaymentAggregate) FailPayment(reason string) error {
	if p.Status == "completed" {
		return fmt.Errorf("cannot fail completed payment")
	}

	eventData := PaymentFailedEventData{
		PaymentID:   p.ID,
		UserID:      p.UserID,
		Amount:      p.Amount,
		Currency:    p.Currency,
		FailureReason: reason,
		FailedAt:    time.Now().UTC(),
	}

	event, err := NewEvent(p.ID, "payment", "PaymentFailed", eventData, p.UserID, uuid.New().String())
	if err != nil {
		return fmt.Errorf("failed to create PaymentFailed event: %w", err)
	}

	p.AddUncommittedEvent(event)
	return p.ApplyEvent(event)
}

// ApplyEvent aplica un evento al agregado
func (p *PaymentAggregate) ApplyEvent(event *Event) error {
	switch event.EventType {
	case "PaymentInitiated":
		var eventData PaymentInitiatedEventData
		if err := event.UnmarshalEventData(&eventData); err != nil {
			return err
		}
		// Apply event data to aggregate state
		p.ID = eventData.PaymentID
		p.UserID = eventData.UserID
		p.Amount = eventData.Amount
		p.Currency = eventData.Currency
		p.PaymentMethod = eventData.PaymentMethod
		p.Status = "initiated"
		
	case "PaymentCompleted":
		var eventData PaymentCompletedEventData
		if err := event.UnmarshalEventData(&eventData); err != nil {
			return err
		}
		p.Status = "completed"
		p.GatewayTxID = eventData.GatewayTxID
		p.CompletedAt = &eventData.CompletedAt
		
	case "PaymentFailed":
		var eventData PaymentFailedEventData
		if err := event.UnmarshalEventData(&eventData); err != nil {
			return err
		}
		p.Status = "failed"
	}

	p.Version++
	return nil
}

// GetType returns the aggregate type
func (p *PaymentAggregate) GetType() string {
	return "payment"
}

// CreateSnapshot creates a snapshot of the payment aggregate
func (p *PaymentAggregate) CreateSnapshot() (json.RawMessage, error) {
	snapshot := struct {
		ID            string     `json:"id"`
		UserID        string     `json:"user_id"`
		Amount        float64    `json:"amount"`
		Currency      string     `json:"currency"`
		PaymentMethod string     `json:"payment_method"`
		Status        string     `json:"status"`
		GatewayTxID   string     `json:"gateway_tx_id,omitempty"`
		CreatedAt     time.Time  `json:"created_at"`
		CompletedAt   *time.Time `json:"completed_at,omitempty"`
		Version       int64      `json:"version"`
	}{
		ID:            p.ID,
		UserID:        p.UserID,
		Amount:        p.Amount,
		Currency:      p.Currency,
		PaymentMethod: p.PaymentMethod,
		Status:        p.Status,
		GatewayTxID:   p.GatewayTxID,
		CreatedAt:     p.CreatedAt,
		CompletedAt:   p.CompletedAt,
		Version:       p.Version,
	}
	
	return json.Marshal(snapshot)
}

// LoadFromSnapshot loads the payment aggregate from a snapshot
func (p *PaymentAggregate) LoadFromSnapshot(data json.RawMessage) error {
	var snapshot struct {
		ID            string     `json:"id"`
		UserID        string     `json:"user_id"`
		Amount        float64    `json:"amount"`
		Currency      string     `json:"currency"`
		PaymentMethod string     `json:"payment_method"`
		Status        string     `json:"status"`
		GatewayTxID   string     `json:"gateway_tx_id,omitempty"`
		CreatedAt     time.Time  `json:"created_at"`
		CompletedAt   *time.Time `json:"completed_at,omitempty"`
		Version       int64      `json:"version"`
	}
	
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return fmt.Errorf("failed to unmarshal payment snapshot: %w", err)
	}
	
	p.ID = snapshot.ID
	p.UserID = snapshot.UserID
	p.Amount = snapshot.Amount
	p.Currency = snapshot.Currency
	p.PaymentMethod = snapshot.PaymentMethod
	p.Status = snapshot.Status
	p.GatewayTxID = snapshot.GatewayTxID
	p.CreatedAt = snapshot.CreatedAt
	p.CompletedAt = snapshot.CompletedAt
	p.Version = snapshot.Version
	
	return nil
}

// WalletAggregate representa un agregado de billetera
type WalletAggregate struct {
	BaseAggregate
	UserID   string  `json:"user_id"`
	Currency string  `json:"currency"`
	Balance  float64 `json:"balance"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// NewWalletAggregate crea un nuevo agregado de billetera
func NewWalletAggregate(id, userID, currency string) *WalletAggregate {
	return &WalletAggregate{
		BaseAggregate: BaseAggregate{
			ID:      id,
			Type:    "wallet",
			Version: 0,
			UncommittedEvents: make([]*Event, 0),
		},
		UserID:    userID,
		Currency:  currency,
		Balance:   0.0,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
}

// AddBalance agrega saldo a la billetera
func (w *WalletAggregate) AddBalance(amount float64, description string) error {
	if amount <= 0 {
		return fmt.Errorf("amount must be positive")
	}

	eventData := BalanceAddedEventData{
		WalletID:        w.ID,
		UserID:          w.UserID,
		Amount:          amount,
		Currency:        w.Currency,
		Description:     description,
		PreviousBalance: w.Balance,
		NewBalance:      w.Balance + amount,
		AddedAt:         time.Now().UTC(),
	}

	event, err := NewEvent(w.ID, "wallet", "BalanceAdded", eventData, w.UserID, uuid.New().String())
	if err != nil {
		return fmt.Errorf("failed to create BalanceAdded event: %w", err)
	}

	w.AddUncommittedEvent(event)
	return w.ApplyEvent(event)
}

// DeductBalance deduce saldo de la billetera
func (w *WalletAggregate) DeductBalance(amount float64, description string) error {
	if amount <= 0 {
		return fmt.Errorf("amount must be positive")
	}

	if w.Balance < amount {
		return fmt.Errorf("insufficient balance: current=%.2f, required=%.2f", w.Balance, amount)
	}

	eventData := BalanceDeductedEventData{
		WalletID:        w.ID,
		UserID:          w.UserID,
		Amount:          amount,
		Currency:        w.Currency,
		Description:     description,
		PreviousBalance: w.Balance,
		NewBalance:      w.Balance - amount,
		DeductedAt:      time.Now().UTC(),
	}

	event, err := NewEvent(w.ID, "wallet", "BalanceDeducted", eventData, w.UserID, uuid.New().String())
	if err != nil {
		return fmt.Errorf("failed to create BalanceDeducted event: %w", err)
	}

	w.AddUncommittedEvent(event)
	return w.ApplyEvent(event)
}

// ApplyEvent aplica un evento al agregado
func (w *WalletAggregate) ApplyEvent(event *Event) error {
	switch event.EventType {
	case "BalanceAdded":
		var eventData BalanceAddedEventData
		if err := event.UnmarshalEventData(&eventData); err != nil {
			return err
		}
		w.Balance = eventData.NewBalance
		w.UpdatedAt = eventData.AddedAt
		
	case "BalanceDeducted":
		var eventData BalanceDeductedEventData
		if err := event.UnmarshalEventData(&eventData); err != nil {
			return err
		}
		w.Balance = eventData.NewBalance
		w.UpdatedAt = eventData.DeductedAt
	}

	w.Version++
	return nil
}

// GetType returns the aggregate type
func (w *WalletAggregate) GetType() string {
	return "wallet"
}

// CreateSnapshot creates a snapshot of the wallet aggregate
func (w *WalletAggregate) CreateSnapshot() (json.RawMessage, error) {
	snapshot := struct {
		ID        string    `json:"id"`
		UserID    string    `json:"user_id"`
		Currency  string    `json:"currency"`
		Balance   float64   `json:"balance"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		Version   int64     `json:"version"`
	}{
		ID:        w.ID,
		UserID:    w.UserID,
		Currency:  w.Currency,
		Balance:   w.Balance,
		CreatedAt: w.CreatedAt,
		UpdatedAt: w.UpdatedAt,
		Version:   w.Version,
	}
	
	return json.Marshal(snapshot)
}

// LoadFromSnapshot loads the wallet aggregate from a snapshot
func (w *WalletAggregate) LoadFromSnapshot(data json.RawMessage) error {
	var snapshot struct {
		ID        string    `json:"id"`
		UserID    string    `json:"user_id"`
		Currency  string    `json:"currency"`
		Balance   float64   `json:"balance"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		Version   int64     `json:"version"`
	}
	
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return fmt.Errorf("failed to unmarshal wallet snapshot: %w", err)
	}
	
	w.ID = snapshot.ID
	w.UserID = snapshot.UserID
	w.Currency = snapshot.Currency
	w.Balance = snapshot.Balance
	w.CreatedAt = snapshot.CreatedAt
	w.UpdatedAt = snapshot.UpdatedAt
	w.Version = snapshot.Version
	
	return nil
}

// Event data structures for Payment aggregate
type PaymentInitiatedEventData struct {
	PaymentID     string    `json:"payment_id"`
	UserID        string    `json:"user_id"`
	Amount        float64   `json:"amount"`
	Currency      string    `json:"currency"`
	PaymentMethod string    `json:"payment_method"`
	InitiatedAt   time.Time `json:"initiated_at"`
}

type PaymentCompletedEventData struct {
	PaymentID            string    `json:"payment_id"`
	UserID               string    `json:"user_id"`
	Amount               float64   `json:"amount"`
	Currency             string    `json:"currency"`
	GatewayTxID          string    `json:"gateway_tx_id"`
	GatewayTransactionID string    `json:"gateway_transaction_id"`
	ProcessingTimeMs     int64     `json:"processing_time_ms"`
	CompletedAt          time.Time `json:"completed_at"`
}

type PaymentFailedEventData struct {
	PaymentID     string    `json:"payment_id"`
	UserID        string    `json:"user_id"`
	Amount        float64   `json:"amount"`
	Currency      string    `json:"currency"`
	FailureReason string    `json:"failure_reason"`
	FailedAt      time.Time `json:"failed_at"`
}

type FundsRevertedEventData struct {
	PaymentID   string    `json:"payment_id"`
	UserID      string    `json:"user_id"`
	Amount      float64   `json:"amount"`
	Currency    string    `json:"currency"`
	SagaID      string    `json:"saga_id"`
	RevertedAt  time.Time `json:"reverted_at"`
	Reason      string    `json:"reason"`
}


// Event data structures for Wallet aggregate
type BalanceAddedEventData struct {
	WalletID        string    `json:"wallet_id"`
	UserID          string    `json:"user_id"`
	Amount          float64   `json:"amount"`
	Currency        string    `json:"currency"`
	Description     string    `json:"description"`
	PreviousBalance float64   `json:"previous_balance"`
	NewBalance      float64   `json:"new_balance"`
	AddedAt         time.Time `json:"added_at"`
}

type BalanceDeductedEventData struct {
	WalletID        string    `json:"wallet_id"`
	UserID          string    `json:"user_id"`
	Amount          float64   `json:"amount"`
	Currency        string    `json:"currency"`
	Description     string    `json:"description"`
	PreviousBalance float64   `json:"previous_balance"`
	NewBalance      float64   `json:"new_balance"`
	DeductedAt      time.Time `json:"deducted_at"`
}

// Aliases for backward compatibility with test files
type WalletCreditedEventData struct {
	TransactionID   string    `json:"transaction_id"`
	Amount          float64   `json:"amount"`
	PreviousBalance float64   `json:"previous_balance"`
	NewBalance      float64   `json:"new_balance"`
	Description     string    `json:"description"`
	CreditedAt      time.Time `json:"credited_at"`
	UserID          string    `json:"user_id"`
	Currency        string    `json:"currency"`
}

type WalletDeductedEventData struct {
	TransactionID   string    `json:"transaction_id"`
	Amount          float64   `json:"amount"`
	PreviousBalance float64   `json:"previous_balance"`
	NewBalance      float64   `json:"new_balance"`
	Description     string    `json:"description"`
	DeductedAt      time.Time `json:"deducted_at"`
	UserID          string    `json:"user_id"`
	Currency        string    `json:"currency"`
}
