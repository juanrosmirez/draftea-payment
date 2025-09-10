package eventstore

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// PaymentSagaStep represents a step in the payment saga
type PaymentSagaStep struct {
	ID          uuid.UUID              `json:"id"`
	Name        string                 `json:"name"`
	Status      string                 `json:"status"`
	Data        map[string]interface{} `json:"data"`
	CreatedAt   time.Time              `json:"created_at"`
	CompletedAt *time.Time             `json:"completed_at,omitempty"`
}

// EventBus defines the interface for event publishing
type EventBus interface {
	Publish(ctx context.Context, event Event) error
	Subscribe(ctx context.Context, eventType string, handler func(Event) error) error
}

// PaymentSaga represents a payment saga orchestration
type PaymentSaga struct {
	ID             uuid.UUID         `json:"id"`
	SagaID         uuid.UUID         `json:"saga_id"`
	PaymentID      string            `json:"payment_id"`
	UserID         string            `json:"user_id"`
	Amount         int64             `json:"amount"`
	Currency       string            `json:"currency"`
	Status         string            `json:"status"`
	Steps          []PaymentSagaStep `json:"steps"`
	CompletedSteps []string          `json:"completed_steps"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
	CompletedAt    *time.Time        `json:"completed_at,omitempty"`
	ErrorMessage   string            `json:"error_message,omitempty"`
}

// PaymentSagaOrchestrator manages payment sagas
type PaymentSagaOrchestrator struct {
	eventStore EventStore
	eventBus   EventBus
}

// NewPaymentSagaOrchestrator creates a new payment saga orchestrator
func NewPaymentSagaOrchestrator(eventStore EventStore, eventBus EventBus) *PaymentSagaOrchestrator {
	return &PaymentSagaOrchestrator{
		eventStore: eventStore,
		eventBus:   eventBus,
	}
}

// StartPaymentSaga starts a new payment saga
func (pso *PaymentSagaOrchestrator) StartPaymentSaga(ctx context.Context, paymentID, userID string) (*PaymentSaga, error) {
	sagaID := uuid.New()
	saga := &PaymentSaga{
		ID:        sagaID,
		PaymentID: paymentID,
		UserID:    userID,
		Status:    "started",
		Steps:     []PaymentSagaStep{},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Create saga started event
	event, err := NewEvent(sagaID.String(), "PaymentSaga", "SagaStarted", map[string]interface{}{
		"saga_id":    sagaID.String(),
		"payment_id": paymentID,
		"user_id":    userID,
		"status":     "started",
	}, userID, fmt.Sprintf("saga-%s", sagaID.String()))
	if err != nil {
		return nil, fmt.Errorf("failed to create saga started event: %w", err)
	}

	if err := pso.eventStore.AppendEvent(ctx, event); err != nil {
		return nil, fmt.Errorf("failed to store saga started event: %w", err)
	}

	return saga, nil
}

// AddStep adds a step to the payment saga
func (pso *PaymentSagaOrchestrator) AddStep(ctx context.Context, sagaID uuid.UUID, stepName string, stepData map[string]interface{}) error {
	stepID := uuid.New()
	step := PaymentSagaStep{
		ID:        stepID,
		Name:      stepName,
		Status:    "pending",
		Data:      stepData,
		CreatedAt: time.Now(),
	}

	// Create step added event using the step struct
	event, err := NewEvent(sagaID.String(), "PaymentSaga", "StepAdded", map[string]interface{}{
		"saga_id":   sagaID.String(),
		"step_id":   step.ID.String(),
		"step_name": step.Name,
		"step_data": step.Data,
		"status":    step.Status,
		"created_at": step.CreatedAt,
	}, "", fmt.Sprintf("saga-%s", sagaID.String()))
	if err != nil {
		return fmt.Errorf("failed to create step added event: %w", err)
	}

	return pso.eventStore.AppendEvent(ctx, event)
}

// CompleteStep marks a saga step as completed
func (pso *PaymentSagaOrchestrator) CompleteStep(ctx context.Context, sagaID, stepID uuid.UUID) error {
	completedAt := time.Now()
	
	// Create step completed event
	event, err := NewEvent(sagaID.String(), "PaymentSaga", "StepCompleted", map[string]interface{}{
		"saga_id":      sagaID.String(),
		"step_id":      stepID.String(),
		"completed_at": completedAt,
		"status":       "completed",
	}, "", fmt.Sprintf("saga-%s", sagaID.String()))
	if err != nil {
		return fmt.Errorf("failed to create step completed event: %w", err)
	}

	return pso.eventStore.AppendEvent(ctx, event)
}

// FailStep marks a saga step as failed
func (pso *PaymentSagaOrchestrator) FailStep(ctx context.Context, sagaID, stepID uuid.UUID, errorMsg string) error {
	failedAt := time.Now()
	
	// Create step failed event
	event, err := NewEvent(sagaID.String(), "PaymentSaga", "StepFailed", map[string]interface{}{
		"saga_id":      sagaID.String(),
		"step_id":      stepID.String(),
		"failed_at":    failedAt,
		"error":        errorMsg,
		"status":       "failed",
	}, "", fmt.Sprintf("saga-%s", sagaID.String()))
	if err != nil {
		return fmt.Errorf("failed to create step failed event: %w", err)
	}

	return pso.eventStore.AppendEvent(ctx, event)
}

// CompensateStep performs compensation for a failed step
func (pso *PaymentSagaOrchestrator) CompensateStep(ctx context.Context, sagaID, stepID uuid.UUID) error {
	compensatedAt := time.Now()
	
	// Create step compensated event
	event, err := NewEvent(sagaID.String(), "PaymentSaga", "StepCompensated", map[string]interface{}{
		"saga_id":        sagaID.String(),
		"step_id":        stepID.String(),
		"compensated_at": compensatedAt,
		"status":         "compensated",
	}, "", fmt.Sprintf("saga-%s", sagaID.String()))
	if err != nil {
		return fmt.Errorf("failed to create step compensated event: %w", err)
	}

	return pso.eventStore.AppendEvent(ctx, event)
}

// CompleteSaga marks the entire saga as completed
func (pso *PaymentSagaOrchestrator) CompleteSaga(ctx context.Context, sagaID uuid.UUID) error {
	completedAt := time.Now()
	
	// Create saga completed event
	event, err := NewEvent(sagaID.String(), "PaymentSaga", "SagaCompleted", map[string]interface{}{
		"saga_id":      sagaID.String(),
		"completed_at": completedAt,
		"status":       "completed",
	}, "", fmt.Sprintf("saga-%s", sagaID.String()))
	if err != nil {
		return fmt.Errorf("failed to create saga completed event: %w", err)
	}

	return pso.eventStore.AppendEvent(ctx, event)
}

// FailSaga marks the entire saga as failed
func (pso *PaymentSagaOrchestrator) FailSaga(ctx context.Context, sagaID uuid.UUID, errorMsg string) error {
	failedAt := time.Now()
	
	// Create saga failed event
	event, err := NewEvent(sagaID.String(), "PaymentSaga", "SagaFailed", map[string]interface{}{
		"saga_id":   sagaID.String(),
		"failed_at": failedAt,
		"error":     errorMsg,
		"status":    "failed",
	}, "", fmt.Sprintf("saga-%s", sagaID.String()))
	if err != nil {
		return fmt.Errorf("failed to create saga failed event: %w", err)
	}

	return pso.eventStore.AppendEvent(ctx, event)
}

// GetSaga retrieves a saga by ID
func (pso *PaymentSagaOrchestrator) GetSaga(ctx context.Context, sagaID uuid.UUID) (*PaymentSaga, error) {
	// This would typically reconstruct the saga from events
	// For now, return a basic implementation
	return &PaymentSaga{
		ID:        sagaID,
		Status:    "unknown",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}, nil
}
