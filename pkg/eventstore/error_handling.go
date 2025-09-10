package eventstore

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/google/uuid"
)

// RetryConfig configura los parámetros de reintento
type RetryConfig struct {
	MaxRetries      int           `json:"max_retries"`
	InitialDelay    time.Duration `json:"initial_delay"`
	MaxDelay        time.Duration `json:"max_delay"`
	BackoffFactor   float64       `json:"backoff_factor"`
	Jitter          bool          `json:"jitter"`
	RetryableErrors []string      `json:"retryable_errors"`
}

// DefaultRetryConfig retorna configuración por defecto
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:    5,
		InitialDelay:  100 * time.Millisecond,
		MaxDelay:      30 * time.Second,
		BackoffFactor: 2.0,
		Jitter:        true,
		RetryableErrors: []string{
			"network_error",
			"timeout",
			"service_unavailable",
			"rate_limit_exceeded",
		},
	}
}

// RetryableOperation define una operación que puede ser reintentada
type RetryableOperation func(ctx context.Context, attempt int) error

// ExponentialBackoffRetrier implementa reintentos con backoff exponencial
type ExponentialBackoffRetrier struct {
	config RetryConfig
	logger Logger
}

type Logger interface {
	Info(msg string, fields ...interface{})
	Warn(msg string, fields ...interface{})
	Error(msg string, fields ...interface{})
}

// NewExponentialBackoffRetrier crea un nuevo retrier
func NewExponentialBackoffRetrier(config RetryConfig, logger Logger) *ExponentialBackoffRetrier {
	return &ExponentialBackoffRetrier{
		config: config,
		logger: logger,
	}
}

// ExecuteWithRetry ejecuta una operación con reintentos y backoff exponencial
func (r *ExponentialBackoffRetrier) ExecuteWithRetry(
	ctx context.Context,
	operation RetryableOperation,
	operationName string,
) error {
	var lastErr error
	
	for attempt := 0; attempt <= r.config.MaxRetries; attempt++ {
		// Crear contexto con timeout para cada intento
		attemptCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		
		r.logger.Info("Executing operation",
			"operation", operationName,
			"attempt", attempt+1,
			"max_attempts", r.config.MaxRetries+1,
		)
		
		// Ejecutar operación
		err := operation(attemptCtx, attempt)
		cancel()
		
		if err == nil {
			// Éxito
			if attempt > 0 {
				r.logger.Info("Operation succeeded after retries",
					"operation", operationName,
					"successful_attempt", attempt+1,
				)
			}
			return nil
		}
		
		lastErr = err
		
		// Verificar si el error es reintentable
		if !r.isRetryableError(err) {
			r.logger.Error("Non-retryable error encountered",
				"operation", operationName,
				"error", err.Error(),
				"attempt", attempt+1,
			)
			return fmt.Errorf("non-retryable error: %w", err)
		}
		
		// Si es el último intento, no esperar
		if attempt == r.config.MaxRetries {
			r.logger.Error("All retry attempts exhausted",
				"operation", operationName,
				"error", err.Error(),
				"total_attempts", attempt+1,
			)
			break
		}
		
		// Calcular delay con backoff exponencial
		delay := r.calculateDelay(attempt)
		
		r.logger.Warn("Operation failed, retrying",
			"operation", operationName,
			"error", err.Error(),
			"attempt", attempt+1,
			"next_retry_in", delay,
		)
		
		// Esperar antes del siguiente intento
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled during retry: %w", ctx.Err())
		case <-time.After(delay):
			// Continuar con el siguiente intento
		}
	}
	
	return fmt.Errorf("operation failed after %d attempts: %w", r.config.MaxRetries+1, lastErr)
}

// calculateDelay calcula el delay con backoff exponencial y jitter opcional
func (r *ExponentialBackoffRetrier) calculateDelay(attempt int) time.Duration {
	// Backoff exponencial: delay = initial_delay * (backoff_factor ^ attempt)
	delay := float64(r.config.InitialDelay) * math.Pow(r.config.BackoffFactor, float64(attempt))
	
	// Aplicar límite máximo
	if delay > float64(r.config.MaxDelay) {
		delay = float64(r.config.MaxDelay)
	}
	
	// Agregar jitter para evitar thundering herd
	if r.config.Jitter {
		// Jitter del 25%: delay ± 25%
		jitterRange := delay * 0.25
		jitter := (2*jitterRange*float64(time.Now().UnixNano()%1000)/1000) - jitterRange
		delay += jitter
	}
	
	return time.Duration(delay)
}

// isRetryableError verifica si un error es reintentable
func (r *ExponentialBackoffRetrier) isRetryableError(err error) bool {
	errorStr := err.Error()
	
	for _, retryableError := range r.config.RetryableErrors {
		if contains(errorStr, retryableError) {
			return true
		}
	}
	
	return false
}

// Dead Letter Queue (DLQ) para eventos fallidos
type DeadLetterQueue struct {
	events    []FailedEvent
	mu        sync.RWMutex
	logger    Logger
	storage   DLQStorage
	maxSize   int
}

type FailedEvent struct {
	OriginalEvent   *Event    `json:"original_event"`
	FailureReason   string    `json:"failure_reason"`
	FailureCount    int       `json:"failure_count"`
	FirstFailedAt   time.Time `json:"first_failed_at"`
	LastFailedAt    time.Time `json:"last_failed_at"`
	LastError       string    `json:"last_error"`
	ProcessingMeta  map[string]interface{} `json:"processing_meta"`
}

type DLQStorage interface {
	Store(ctx context.Context, failedEvent FailedEvent) error
	Retrieve(ctx context.Context, limit int) ([]FailedEvent, error)
	Remove(ctx context.Context, eventID string) error
}

// NewDeadLetterQueue crea una nueva DLQ
func NewDeadLetterQueue(logger Logger, storage DLQStorage, maxSize int) *DeadLetterQueue {
	return &DeadLetterQueue{
		events:  make([]FailedEvent, 0),
		logger:  logger,
		storage: storage,
		maxSize: maxSize,
	}
}

// SendToDLQ envía un evento fallido a la Dead Letter Queue
func (dlq *DeadLetterQueue) SendToDLQ(
	ctx context.Context,
	event *Event,
	failureReason string,
	lastError error,
	failureCount int,
) error {
	dlq.mu.Lock()
	defer dlq.mu.Unlock()
	
	failedEvent := FailedEvent{
		OriginalEvent: event,
		FailureReason: failureReason,
		FailureCount:  failureCount,
		FirstFailedAt: time.Now().UTC(),
		LastFailedAt:  time.Now().UTC(),
		LastError:     lastError.Error(),
		ProcessingMeta: map[string]interface{}{
			"dlq_timestamp": time.Now().UTC(),
			"correlation_id": event.CorrelationID,
			"aggregate_id": event.AggregateID,
			"event_type": event.EventType,
		},
	}
	
	// Buscar si ya existe el evento en DLQ
	for i, existing := range dlq.events {
		if existing.OriginalEvent.ID == event.ID {
			// Actualizar evento existente
			dlq.events[i].FailureCount = failureCount
			dlq.events[i].LastFailedAt = time.Now().UTC()
			dlq.events[i].LastError = lastError.Error()
			
			dlq.logger.Warn("Updated existing event in DLQ",
				"event_id", event.ID,
				"event_type", event.EventType,
				"failure_count", failureCount,
				"failure_reason", failureReason,
			)
			
			return dlq.storage.Store(ctx, dlq.events[i])
		}
	}
	
	// Agregar nuevo evento fallido
	dlq.events = append(dlq.events, failedEvent)
	
	// Verificar límite de tamaño
	if len(dlq.events) > dlq.maxSize {
		// Remover eventos más antiguos
		dlq.events = dlq.events[1:]
		dlq.logger.Warn("DLQ size limit reached, removing oldest event")
	}
	
	dlq.logger.Error("Event sent to Dead Letter Queue",
		"event_id", event.ID,
		"event_type", event.EventType,
		"aggregate_id", event.AggregateID,
		"failure_reason", failureReason,
		"failure_count", failureCount,
		"last_error", lastError.Error(),
	)
	
	// Persistir en storage
	if err := dlq.storage.Store(ctx, failedEvent); err != nil {
		dlq.logger.Error("Failed to persist event to DLQ storage",
			"event_id", event.ID,
			"storage_error", err.Error(),
		)
		return fmt.Errorf("failed to persist to DLQ storage: %w", err)
	}
	
	return nil
}

// GetFailedEvents retorna eventos en la DLQ
func (dlq *DeadLetterQueue) GetFailedEvents(ctx context.Context, limit int) ([]FailedEvent, error) {
	dlq.mu.RLock()
	defer dlq.mu.RUnlock()
	
	if limit <= 0 || limit > len(dlq.events) {
		limit = len(dlq.events)
	}
	
	return dlq.events[:limit], nil
}

// RetryFromDLQ intenta reprocesar un evento desde la DLQ
func (dlq *DeadLetterQueue) RetryFromDLQ(
	ctx context.Context,
	eventID string,
	processor func(context.Context, *Event) error,
) error {
	dlq.mu.Lock()
	defer dlq.mu.Unlock()
	
	for i, failedEvent := range dlq.events {
		if failedEvent.OriginalEvent.ID == eventID {
			dlq.logger.Info("Retrying event from DLQ",
				"event_id", eventID,
				"event_type", failedEvent.OriginalEvent.EventType,
				"previous_failures", failedEvent.FailureCount,
			)
			
			// Intentar reprocesar
			if err := processor(ctx, failedEvent.OriginalEvent); err != nil {
				dlq.logger.Error("Failed to retry event from DLQ",
					"event_id", eventID,
					"error", err.Error(),
				)
				return fmt.Errorf("failed to retry event: %w", err)
			}
			
			// Remover de DLQ si fue exitoso
			dlq.events = append(dlq.events[:i], dlq.events[i+1:]...)
			
			// Remover del storage
			if err := dlq.storage.Remove(ctx, eventID); err != nil {
				dlq.logger.Warn("Failed to remove event from DLQ storage",
					"event_id", eventID,
					"error", err.Error(),
				)
			}
			
			dlq.logger.Info("Successfully retried event from DLQ",
				"event_id", eventID,
			)
			
			return nil
		}
	}
	
	return fmt.Errorf("event not found in DLQ: %s", eventID)
}

// CompensationManager maneja la lógica de compensación de transacciones
type CompensationManager struct {
	eventStore      EventStore
	eventBus        EventBus
	walletService   WalletServiceInterface
	retrier         *ExponentialBackoffRetrier
	dlq             *DeadLetterQueue
	logger          Logger
	
	// Registro de compensaciones ejecutadas
	compensations   map[string]CompensationRecord
	mu              sync.RWMutex
}

type CompensationRecord struct {
	SagaID            string                 `json:"saga_id"`
	PaymentID         string                 `json:"payment_id"`
	CompensationSteps []CompensationStep     `json:"compensation_steps"`
	Status            string                 `json:"status"`
	CreatedAt         time.Time              `json:"created_at"`
	CompletedAt       *time.Time             `json:"completed_at,omitempty"`
	Metadata          map[string]interface{} `json:"metadata"`
}

type CompensationStep struct {
	StepName      string    `json:"step_name"`
	Action        string    `json:"action"`
	Status        string    `json:"status"`
	ExecutedAt    time.Time `json:"executed_at"`
	Error         string    `json:"error,omitempty"`
	RetryCount    int       `json:"retry_count"`
}

// NewCompensationManager crea un nuevo gestor de compensaciones
func NewCompensationManager(
	eventStore EventStore,
	eventBus EventBus,
	walletService WalletServiceInterface,
	retrier *ExponentialBackoffRetrier,
	dlq *DeadLetterQueue,
	logger Logger,
) *CompensationManager {
	return &CompensationManager{
		eventStore:    eventStore,
		eventBus:      eventBus,
		walletService: walletService,
		retrier:       retrier,
		dlq:           dlq,
		logger:        logger,
		compensations: make(map[string]CompensationRecord),
	}
}

// CompensatePaymentFailure ejecuta compensación completa para un pago fallido
func (cm *CompensationManager) CompensatePaymentFailure(
	ctx context.Context,
	saga *PaymentSaga,
	failureReason string,
	originalError error,
) error {
	compensationID := uuid.New().String()
	
	cm.logger.Info("Starting payment compensation",
		"saga_id", saga.SagaID,
		"payment_id", saga.PaymentID,
		"compensation_id", compensationID,
		"failure_reason", failureReason,
	)
	
	// Crear registro de compensación
	compensation := CompensationRecord{
		SagaID:    saga.SagaID.String(),
		PaymentID: saga.PaymentID,
		Status:    "in_progress",
		CreatedAt: time.Now().UTC(),
		Metadata: map[string]interface{}{
			"compensation_id": compensationID,
			"failure_reason":  failureReason,
			"original_error":  originalError.Error(),
		},
	}
	
	cm.mu.Lock()
	cm.compensations[compensationID] = compensation
	cm.mu.Unlock()
	
	// Ejecutar pasos de compensación en orden inverso
	compensationSteps := cm.buildCompensationSteps(saga)
	
	for _, step := range compensationSteps {
		if err := cm.executeCompensationStep(ctx, saga, step, compensationID); err != nil {
			cm.logger.Error("Compensation step failed",
				"saga_id", saga.SagaID,
				"step", step.StepName,
				"error", err.Error(),
			)
			
			// Marcar compensación como fallida
			cm.updateCompensationStatus(compensationID, "failed")
			return fmt.Errorf("compensation failed at step %s: %w", step.StepName, err)
		}
	}
	
	// Marcar compensación como completada
	cm.updateCompensationStatus(compensationID, "completed")
	
	cm.logger.Info("Payment compensation completed successfully",
		"saga_id", saga.SagaID,
		"payment_id", saga.PaymentID,
		"compensation_id", compensationID,
	)
	
	return nil
}

// buildCompensationSteps construye los pasos de compensación basados en el estado de la saga
func (cm *CompensationManager) buildCompensationSteps(saga *PaymentSaga) []CompensationStep {
	var steps []CompensationStep
	
	// Revisar pasos completados y crear compensaciones en orden inverso
	for i := len(saga.CompletedSteps) - 1; i >= 0; i-- {
		completedStep := saga.CompletedSteps[i]
		
		switch completedStep {
		case "funds_deducted":
			steps = append(steps, CompensationStep{
				StepName: "revert_funds_deduction",
				Action:   "refund_funds",
				Status:   "pending",
			})
		case "external_processing":
			steps = append(steps, CompensationStep{
				StepName: "cancel_external_payment",
				Action:   "cancel_gateway_payment",
				Status:   "pending",
			})
		}
	}
	
	return steps
}

// executeCompensationStep ejecuta un paso individual de compensación
func (cm *CompensationManager) executeCompensationStep(
	ctx context.Context,
	saga *PaymentSaga,
	step CompensationStep,
	compensationID string,
) error {
	cm.logger.Info("Executing compensation step",
		"saga_id", saga.SagaID,
		"step", step.StepName,
		"action", step.Action,
	)
	
	step.ExecutedAt = time.Now().UTC()
	
	// Ejecutar acción con reintentos
	operation := func(ctx context.Context, attempt int) error {
		switch step.Action {
		case "refund_funds":
			return cm.executeRefundFunds(ctx, saga, step, attempt)
		case "cancel_gateway_payment":
			return cm.executeCancelGatewayPayment(ctx, saga, step, attempt)
		default:
			return fmt.Errorf("unknown compensation action: %s", step.Action)
		}
	}
	
	err := cm.retrier.ExecuteWithRetry(ctx, operation, step.StepName)
	if err != nil {
		step.Status = "failed"
		step.Error = err.Error()
		
		// Enviar a DLQ si falla definitivamente
		stepData := map[string]interface{}{
			"step_name":    step.StepName,
			"action":       step.Action,
			"status":       step.Status,
			"executed_at":  step.ExecutedAt,
			"error":        step.Error,
			"retry_count":  step.RetryCount,
		}
		
		eventDataBytes, err := json.Marshal(stepData)
		if err != nil {
			return fmt.Errorf("failed to marshal compensation step data: %w", err)
		}
		
		compensationEvent := &Event{
			ID:            uuid.New().String(),
			AggregateID:   saga.PaymentID,
			AggregateType: "compensation",
			EventType:     "CompensationStepFailed",
			EventData:     json.RawMessage(eventDataBytes),
			CreatedAt:     time.Now().UTC(),
			CorrelationID: saga.SagaID.String(),
		}
		
		cm.dlq.SendToDLQ(ctx, compensationEvent, "compensation_step_failed", err, step.RetryCount)
		return err
	}
	
	step.Status = "completed"
	
	// Actualizar registro de compensación
	cm.updateCompensationStep(compensationID, step)
	
	return nil
}

// executeRefundFunds ejecuta la reversión de fondos
func (cm *CompensationManager) executeRefundFunds(
	ctx context.Context,
	saga *PaymentSaga,
	step CompensationStep,
	attempt int,
) error {
	_ = step // Step parameter not used in this implementation but kept for interface consistency
	cm.logger.Info("Executing funds refund",
		"saga_id", saga.SagaID,
		"user_id", saga.UserID,
		"amount", saga.Amount,
		"currency", saga.Currency,
		"attempt", attempt+1,
	)
	
	// Ejecutar refund con el wallet service
	err := cm.walletService.RefundFunds(
		ctx,
		saga.UserID,
		saga.Currency,
		float64(saga.Amount),
		fmt.Sprintf("Compensation for failed payment %s", saga.PaymentID),
	)
	if err != nil {
		return fmt.Errorf("failed to refund funds: %w", err)
	}
	
	// Crear y publicar evento de compensación
	fundsRevertedEvent, err := NewEvent(
		saga.UserID,
		"PaymentSaga",
		"FundsReverted",
		FundsRevertedEventData{
			PaymentID:  saga.PaymentID,
			UserID:     saga.UserID,
			Amount:     float64(saga.Amount),
			Currency:   saga.Currency,
			SagaID:     saga.SagaID.String(),
			Reason:     "Payment compensation - external payment failed",
			RevertedAt: time.Now().UTC(),
		},
		saga.UserID,
		saga.SagaID.String(),
	)
	if err != nil {
		return fmt.Errorf("failed to create FundsReverted event: %w", err)
	}
	
	// Persistir evento
	if err := cm.eventStore.AppendEvent(ctx, fundsRevertedEvent); err != nil {
		return fmt.Errorf("failed to store FundsReverted event: %w", err)
	}
	
	// Publicar evento
	if err := cm.eventBus.Publish(ctx, *fundsRevertedEvent); err != nil {
		return fmt.Errorf("failed to publish FundsReverted event: %w", err)
	}
	
	cm.logger.Info("Funds refund completed successfully",
		"saga_id", saga.SagaID,
		"amount", saga.Amount,
		"currency", saga.Currency,
	)
	
	return nil
}

// executeCancelGatewayPayment cancela el pago en la pasarela externa
func (cm *CompensationManager) executeCancelGatewayPayment(
	ctx context.Context,
	saga *PaymentSaga,
	step CompensationStep,
	attempt int,
) error {
	_ = ctx  // Context not used in this implementation but kept for interface consistency
	_ = step // Step parameter not used in this implementation but kept for interface consistency
	cm.logger.Info("Executing gateway payment cancellation",
		"saga_id", saga.SagaID,
		"payment_id", saga.PaymentID,
		"attempt", attempt+1,
	)
	
	// En una implementación real, aquí se llamaría a la API de la pasarela
	// para cancelar o revertir el pago
	
	// Simular llamada a pasarela externa
	time.Sleep(100 * time.Millisecond)
	
	cm.logger.Info("Gateway payment cancellation completed",
		"saga_id", saga.SagaID,
		"payment_id", saga.PaymentID,
	)
	
	return nil
}

// updateCompensationStatus actualiza el estado de una compensación
func (cm *CompensationManager) updateCompensationStatus(compensationID, status string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	
	if compensation, exists := cm.compensations[compensationID]; exists {
		compensation.Status = status
		if status == "completed" || status == "failed" {
			now := time.Now().UTC()
			compensation.CompletedAt = &now
		}
		cm.compensations[compensationID] = compensation
	}
}

// updateCompensationStep actualiza un paso de compensación
func (cm *CompensationManager) updateCompensationStep(compensationID string, step CompensationStep) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	
	if compensation, exists := cm.compensations[compensationID]; exists {
		// Buscar y actualizar el paso
		for i, existingStep := range compensation.CompensationSteps {
			if existingStep.StepName == step.StepName {
				compensation.CompensationSteps[i] = step
				cm.compensations[compensationID] = compensation
				return
			}
		}
		// Si no existe, agregarlo
		compensation.CompensationSteps = append(compensation.CompensationSteps, step)
		cm.compensations[compensationID] = compensation
	}
}

// Función auxiliar
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || 
		(len(s) > len(substr) && 
			(s[:len(substr)] == substr || 
			 s[len(s)-len(substr):] == substr ||
			 containsSubstring(s, substr))))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
