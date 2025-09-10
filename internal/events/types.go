package events

import (
	"time"

	"github.com/google/uuid"
)

// EventType representa los tipos de eventos del sistema
type EventType string

const (
	// Payment Events
	PaymentInitiated EventType = "payment.initiated"
	PaymentCompleted EventType = "payment.completed"
	PaymentFailed    EventType = "payment.failed"
	PaymentCancelled EventType = "payment.cancelled"

	// Wallet Events
	WalletDeducted   EventType = "wallet.deducted"
	WalletRefunded   EventType = "wallet.refunded"
	WalletValidated  EventType = "wallet.validated"
	WalletInsufficient EventType = "wallet.insufficient_funds"

	// Gateway Events
	GatewayRequestSent     EventType = "gateway.request_sent"
	GatewayResponseReceived EventType = "gateway.response_received"
	GatewayTimeout         EventType = "gateway.timeout"
	GatewayError          EventType = "gateway.error"

	// Metrics Events
	MetricsCollected EventType = "metrics.collected"
	AlertTriggered   EventType = "alert.triggered"
)

// BaseEvent estructura base para todos los eventos
type BaseEvent struct {
	ID          uuid.UUID   `json:"id"`
	Type        EventType   `json:"type"`
	AggregateID uuid.UUID   `json:"aggregate_id"`
	Version     int         `json:"version"`
	Timestamp   time.Time   `json:"timestamp"`
	CorrelationID uuid.UUID `json:"correlation_id"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// PaymentInitiatedEvent evento cuando se inicia un pago
type PaymentInitiatedEvent struct {
	BaseEvent
	UserID      uuid.UUID `json:"user_id"`
	Amount      int64     `json:"amount"` // en centavos
	Currency    string    `json:"currency"`
	ServiceID   string    `json:"service_id"`
	Description string    `json:"description"`
}

// PaymentCompletedEvent evento cuando se completa un pago
type PaymentCompletedEvent struct {
	BaseEvent
	UserID        uuid.UUID `json:"user_id"`
	Amount        int64     `json:"amount"`
	Currency      string    `json:"currency"`
	GatewayTxnID  string    `json:"gateway_txn_id"`
	ProcessedAt   time.Time `json:"processed_at"`
}

// PaymentFailedEvent evento cuando falla un pago
type PaymentFailedEvent struct {
	BaseEvent
	UserID    uuid.UUID `json:"user_id"`
	Amount    int64     `json:"amount"`
	Currency  string    `json:"currency"`
	Reason    string    `json:"reason"`
	ErrorCode string    `json:"error_code"`
}

// WalletDeductedEvent evento cuando se deduce saldo de billetera
type WalletDeductedEvent struct {
	BaseEvent
	UserID        uuid.UUID `json:"user_id"`
	Amount        int64     `json:"amount"`
	Currency      string    `json:"currency"`
	PreviousBalance int64   `json:"previous_balance"`
	NewBalance    int64     `json:"new_balance"`
	PaymentID     uuid.UUID `json:"payment_id"`
}

// WalletRefundedEvent evento cuando se reembolsa saldo a billetera
type WalletRefundedEvent struct {
	BaseEvent
	UserID        uuid.UUID `json:"user_id"`
	Amount        int64     `json:"amount"`
	Currency      string    `json:"currency"`
	PreviousBalance int64   `json:"previous_balance"`
	NewBalance    int64     `json:"new_balance"`
	PaymentID     uuid.UUID `json:"payment_id"`
	Reason        string    `json:"reason"`
}

// GatewayResponseReceivedEvent evento cuando se recibe respuesta de pasarela
type GatewayResponseReceivedEvent struct {
	BaseEvent
	PaymentID     uuid.UUID `json:"payment_id"`
	GatewayTxnID  string    `json:"gateway_txn_id"`
	Status        string    `json:"status"`
	ResponseCode  string    `json:"response_code"`
	ResponseData  map[string]interface{} `json:"response_data"`
	ProcessingTime time.Duration `json:"processing_time"`
}

// MetricsCollectedEvent evento para métricas recopiladas
type MetricsCollectedEvent struct {
	BaseEvent
	MetricType  string                 `json:"metric_type"`
	MetricName  string                 `json:"metric_name"`
	Value       float64                `json:"value"`
	Labels      map[string]string      `json:"labels"`
	Dimensions  map[string]interface{} `json:"dimensions"`
}
