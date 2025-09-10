package saga

import (
	"context"
	"fmt"
	"time"

	"payment-system/internal/events"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// SagaState estados de una saga
type SagaState string

const (
	StateStarted     SagaState = "started"
	StateInProgress  SagaState = "in_progress"
	StateCompleted   SagaState = "completed"
	StateFailed      SagaState = "failed"
	StateCompensated SagaState = "compensated"
)

// StepStatus estados de un paso de saga
type StepStatus string

const (
	StepPending     StepStatus = "pending"
	StepExecuting   StepStatus = "executing"
	StepCompleted   StepStatus = "completed"
	StepFailed      StepStatus = "failed"
	StepCompensated StepStatus = "compensated"
)

// SagaStep representa un paso en una saga
type SagaStep struct {
	ID               uuid.UUID              `json:"id"`
	Name             string                 `json:"name"`
	ServiceName      string                 `json:"service_name"`
	Action           string                 `json:"action"`
	CompensateAction string                 `json:"compensate_action"`
	Status           StepStatus             `json:"status"`
	Input            map[string]interface{} `json:"input"`
	Output           map[string]interface{} `json:"output"`
	Error            string                 `json:"error,omitempty"`
	ExecutedAt       *time.Time             `json:"executed_at,omitempty"`
	CompensatedAt    *time.Time             `json:"compensated_at,omitempty"`
	RetryCount       int                    `json:"retry_count"`
	MaxRetries       int                    `json:"max_retries"`
}

// Saga representa una transacción distribuida
type Saga struct {
	ID            uuid.UUID              `json:"id"`
	Type          string                 `json:"type"`
	State         SagaState              `json:"state"`
	Steps         []*SagaStep            `json:"steps"`
	CurrentStep   int                    `json:"current_step"`
	Context       map[string]interface{} `json:"context"`
	CreatedAt     time.Time              `json:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at"`
	CompletedAt   *time.Time             `json:"completed_at,omitempty"`
	CorrelationID uuid.UUID              `json:"correlation_id"`
	UserID        uuid.UUID              `json:"user_id"`
}

// Repository interfaz para persistencia de sagas
type Repository interface {
	SaveSaga(ctx context.Context, saga *Saga) error
	GetSaga(ctx context.Context, id uuid.UUID) (*Saga, error)
	UpdateSagaState(ctx context.Context, id uuid.UUID, state SagaState) error
	UpdateStepStatus(ctx context.Context, sagaID uuid.UUID, stepID uuid.UUID, status StepStatus, output map[string]interface{}, error string) error
	GetPendingSagas(ctx context.Context) ([]*Saga, error)
}

// StepExecutor interfaz para ejecutar pasos de saga
type StepExecutor interface {
	Execute(ctx context.Context, step *SagaStep) (map[string]interface{}, error)
	Compensate(ctx context.Context, step *SagaStep) error
}

// Orchestrator orquestador de sagas
type Orchestrator struct {
	repo            Repository
	eventPublisher  events.Publisher
	eventSubscriber events.Subscriber
	stepExecutors   map[string]StepExecutor
	logger          *zap.Logger
}

// NewOrchestrator crea un nuevo orquestador de sagas
func NewOrchestrator(
	repo Repository,
	eventPublisher events.Publisher,
	eventSubscriber events.Subscriber,
	logger *zap.Logger,
) *Orchestrator {
	return &Orchestrator{
		repo:            repo,
		eventPublisher:  eventPublisher,
		eventSubscriber: eventSubscriber,
		stepExecutors:   make(map[string]StepExecutor),
		logger:          logger,
	}
}

// RegisterStepExecutor registra un ejecutor para un servicio específico
func (o *Orchestrator) RegisterStepExecutor(serviceName string, executor StepExecutor) {
	o.stepExecutors[serviceName] = executor
	o.logger.Info("step executor registered", zap.String("service", serviceName))
}

// StartPaymentSaga inicia una saga de pago
func (o *Orchestrator) StartPaymentSaga(ctx context.Context, paymentID, userID uuid.UUID, amount int64, currency string) (*Saga, error) {
	sagaID := uuid.New()
	correlationID := uuid.New()

	saga := &Saga{
		ID:    sagaID,
		Type:  "payment",
		State: StateStarted,
		Steps: []*SagaStep{
			{
				ID:               uuid.New(),
				Name:             "validate_wallet_balance",
				ServiceName:      "wallet",
				Action:           "validate_balance",
				CompensateAction: "",
				Status:           StepPending,
				Input: map[string]interface{}{
					"user_id":  userID.String(),
					"amount":   amount,
					"currency": currency,
				},
				MaxRetries: 3,
			},
			{
				ID:               uuid.New(),
				Name:             "deduct_wallet_balance",
				ServiceName:      "wallet",
				Action:           "deduct_balance",
				CompensateAction: "refund_balance",
				Status:           StepPending,
				Input: map[string]interface{}{
					"user_id":    userID.String(),
					"amount":     amount,
					"currency":   currency,
					"payment_id": paymentID.String(),
				},
				MaxRetries: 3,
			},
			{
				ID:               uuid.New(),
				Name:             "process_gateway_payment",
				ServiceName:      "gateway",
				Action:           "process_payment",
				CompensateAction: "",
				Status:           StepPending,
				Input: map[string]interface{}{
					"payment_id": paymentID.String(),
					"amount":     amount,
					"currency":   currency,
					"user_id":    userID.String(),
				},
				MaxRetries: 2,
			},
			{
				ID:               uuid.New(),
				Name:             "update_payment_status",
				ServiceName:      "payment",
				Action:           "update_status",
				CompensateAction: "mark_failed",
				Status:           StepPending,
				Input: map[string]interface{}{
					"payment_id": paymentID.String(),
					"status":     "completed",
				},
				MaxRetries: 3,
			},
		},
		CurrentStep: 0,
		Context: map[string]interface{}{
			"payment_id": paymentID.String(),
			"user_id":    userID.String(),
			"amount":     amount,
			"currency":   currency,
		},
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		CorrelationID: correlationID,
		UserID:        userID,
	}

	if err := o.repo.SaveSaga(ctx, saga); err != nil {
		return nil, fmt.Errorf("failed to save saga: %w", err)
	}

	o.logger.Info("payment saga started",
		zap.String("saga_id", sagaID.String()),
		zap.String("payment_id", paymentID.String()),
		zap.String("user_id", userID.String()))

	// Iniciar ejecución de la saga
	go o.executeSaga(context.Background(), saga)

	return saga, nil
}

// executeSaga ejecuta una saga paso a paso
func (o *Orchestrator) executeSaga(ctx context.Context, saga *Saga) {
	o.logger.Info("executing saga", zap.String("saga_id", saga.ID.String()))

	saga.State = StateInProgress
	o.repo.UpdateSagaState(ctx, saga.ID, StateInProgress)

	for saga.CurrentStep < len(saga.Steps) {
		step := saga.Steps[saga.CurrentStep]

		o.logger.Info("executing saga step",
			zap.String("saga_id", saga.ID.String()),
			zap.String("step_name", step.Name),
			zap.Int("step_number", saga.CurrentStep+1))

		if err := o.executeStep(ctx, saga, step); err != nil {
			o.logger.Error("saga step failed",
				zap.String("saga_id", saga.ID.String()),
				zap.String("step_name", step.Name),
				zap.Error(err))

			// Iniciar compensación
			if err := o.compensateSaga(ctx, saga); err != nil {
				o.logger.Error("saga compensation failed",
					zap.String("saga_id", saga.ID.String()),
					zap.Error(err))
			}
			return
		}

		saga.CurrentStep++
	}

	// Saga completada exitosamente
	saga.State = StateCompleted
	now := time.Now()
	saga.CompletedAt = &now
	saga.UpdatedAt = now

	o.repo.UpdateSagaState(ctx, saga.ID, StateCompleted)

	o.logger.Info("saga completed successfully",
		zap.String("saga_id", saga.ID.String()),
		zap.Duration("duration", time.Since(saga.CreatedAt)))
}

// executeStep ejecuta un paso individual de la saga
func (o *Orchestrator) executeStep(ctx context.Context, saga *Saga, step *SagaStep) error {
	executor, exists := o.stepExecutors[step.ServiceName]
	if !exists {
		return fmt.Errorf("no executor found for service: %s", step.ServiceName)
	}

	step.Status = StepExecuting
	now := time.Now()
	step.ExecutedAt = &now

	o.repo.UpdateStepStatus(ctx, saga.ID, step.ID, StepExecuting, nil, "")

	// Intentar ejecutar con reintentos
	var lastErr error
	for attempt := 0; attempt <= step.MaxRetries; attempt++ {
		if attempt > 0 {
			// Esperar antes del reintento con backoff exponencial
			backoff := time.Duration(attempt*attempt) * time.Second
			o.logger.Info("retrying step execution",
				zap.String("step_name", step.Name),
				zap.Int("attempt", attempt),
				zap.Duration("backoff", backoff))

			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		output, err := executor.Execute(ctx, step)
		if err == nil {
			step.Status = StepCompleted
			step.Output = output
			step.RetryCount = attempt

			o.repo.UpdateStepStatus(ctx, saga.ID, step.ID, StepCompleted, output, "")

			o.logger.Info("step executed successfully",
				zap.String("step_name", step.Name),
				zap.Int("attempts", attempt+1))

			return nil
		}

		lastErr = err
		step.RetryCount = attempt + 1

		o.logger.Warn("step execution failed",
			zap.String("step_name", step.Name),
			zap.Int("attempt", attempt+1),
			zap.Error(err))
	}

	// Todos los reintentos fallaron
	step.Status = StepFailed
	step.Error = lastErr.Error()

	o.repo.UpdateStepStatus(ctx, saga.ID, step.ID, StepFailed, nil, lastErr.Error())

	return fmt.Errorf("step failed after %d attempts: %w", step.MaxRetries+1, lastErr)
}

// compensateSaga ejecuta la compensación de una saga fallida
func (o *Orchestrator) compensateSaga(ctx context.Context, saga *Saga) error {
	o.logger.Info("starting saga compensation", zap.String("saga_id", saga.ID.String()))

	saga.State = StateFailed
	o.repo.UpdateSagaState(ctx, saga.ID, StateFailed)

	// Compensar pasos en orden inverso, solo los que se completaron
	for i := saga.CurrentStep - 1; i >= 0; i-- {
		step := saga.Steps[i]

		if step.Status != StepCompleted || step.CompensateAction == "" {
			continue
		}

		o.logger.Info("compensating step",
			zap.String("saga_id", saga.ID.String()),
			zap.String("step_name", step.Name))

		executor, exists := o.stepExecutors[step.ServiceName]
		if !exists {
			o.logger.Error("no executor found for compensation",
				zap.String("service", step.ServiceName))
			continue
		}

		if err := executor.Compensate(ctx, step); err != nil {
			o.logger.Error("step compensation failed",
				zap.String("step_name", step.Name),
				zap.Error(err))
			// Continuar con otros pasos de compensación
		} else {
			step.Status = StepCompensated
			now := time.Now()
			step.CompensatedAt = &now

			o.repo.UpdateStepStatus(ctx, saga.ID, step.ID, StepCompensated, nil, "")

			o.logger.Info("step compensated successfully",
				zap.String("step_name", step.Name))
		}
	}

	saga.State = StateCompensated
	o.repo.UpdateSagaState(ctx, saga.ID, StateCompensated)

	o.logger.Info("saga compensation completed", zap.String("saga_id", saga.ID.String()))
	return nil
}

// RecoverPendingSagas recupera sagas pendientes después de un reinicio
func (o *Orchestrator) RecoverPendingSagas(ctx context.Context) error {
	o.logger.Info("recovering pending sagas")

	sagas, err := o.repo.GetPendingSagas(ctx)
	if err != nil {
		return fmt.Errorf("failed to get pending sagas: %w", err)
	}

	for _, saga := range sagas {
		o.logger.Info("recovering saga",
			zap.String("saga_id", saga.ID.String()),
			zap.String("state", string(saga.State)))

		switch saga.State {
		case StateInProgress:
			// Continuar ejecución desde el paso actual
			go o.executeSaga(context.Background(), saga)
		case StateFailed:
			// Intentar compensación si no se completó
			if saga.State != StateCompensated {
				go o.compensateSaga(context.Background(), saga)
			}
		}
	}

	o.logger.Info("saga recovery completed", zap.Int("count", len(sagas)))
	return nil
}

// GetSaga obtiene una saga por ID
func (o *Orchestrator) GetSaga(ctx context.Context, id uuid.UUID) (*Saga, error) {
	return o.repo.GetSaga(ctx, id)
}

// Start inicia el orquestador
func (o *Orchestrator) Start(ctx context.Context) error {
	// Suscribirse a eventos relevantes
	o.eventSubscriber.Subscribe(ctx, events.PaymentInitiated, o.handlePaymentInitiated)
	o.eventSubscriber.Subscribe(ctx, events.PaymentFailed, o.handlePaymentFailed)

	// Recuperar sagas pendientes
	if err := o.RecoverPendingSagas(ctx); err != nil {
		return fmt.Errorf("failed to recover pending sagas: %w", err)
	}

	return o.eventSubscriber.Start(ctx)
}

// handlePaymentInitiated maneja eventos de pago iniciado
func (o *Orchestrator) handlePaymentInitiated(ctx context.Context, event interface{}) error {
	paymentEvent, ok := event.(events.PaymentInitiatedEvent)
	if !ok {
		return fmt.Errorf("invalid event type")
	}

	// La saga ya se inicia desde el servicio de pagos
	// Aquí podríamos agregar lógica adicional si es necesario

	o.logger.Debug("payment initiated event received",
		zap.String("payment_id", paymentEvent.AggregateID.String()))

	return nil
}

// handlePaymentFailed maneja eventos de pago fallido
func (o *Orchestrator) handlePaymentFailed(ctx context.Context, event interface{}) error {
	paymentEvent, ok := event.(events.PaymentFailedEvent)
	if !ok {
		return fmt.Errorf("invalid event type")
	}

	o.logger.Info("payment failed event received, checking for saga compensation",
		zap.String("payment_id", paymentEvent.AggregateID.String()))

	// Aquí podríamos buscar sagas relacionadas y activar compensación si es necesario

	return nil
}
