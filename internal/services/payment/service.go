package payment

import (
	"context"
	"fmt"
	"time"

	"payment-system/internal/events"
	"payment-system/internal/saga"
	
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// PaymentRequest estructura para solicitudes de pago
type PaymentRequest struct {
	UserID      uuid.UUID `json:"user_id"`
	Amount      int64     `json:"amount"`
	Currency    string    `json:"currency"`
	ServiceID   string    `json:"service_id"`
	Description string    `json:"description"`
}

// PaymentStatus estados posibles de un pago
type PaymentStatus string

const (
	StatusPending   PaymentStatus = "pending"
	StatusProcessing PaymentStatus = "processing"
	StatusCompleted PaymentStatus = "completed"
	StatusFailed    PaymentStatus = "failed"
	StatusCancelled PaymentStatus = "cancelled"
)

// Payment entidad de pago
type Payment struct {
	ID            uuid.UUID     `json:"id"`
	UserID        uuid.UUID     `json:"user_id"`
	Amount        int64         `json:"amount"`
	Currency      string        `json:"currency"`
	ServiceID     string        `json:"service_id"`
	Description   string        `json:"description"`
	Status        PaymentStatus `json:"status"`
	GatewayTxnID  string        `json:"gateway_txn_id,omitempty"`
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`
	CorrelationID uuid.UUID     `json:"correlation_id"`
}

// Repository interfaz para persistencia de pagos
type Repository interface {
	Save(ctx context.Context, payment *Payment) error
	GetByID(ctx context.Context, id uuid.UUID) (*Payment, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status PaymentStatus) error
}

// WalletService interfaz para interactuar con el servicio de billetera
type WalletService interface {
	ValidateBalance(ctx context.Context, userID uuid.UUID, amount int64, currency string) error
	DeductBalance(ctx context.Context, userID uuid.UUID, amount int64, currency string, paymentID uuid.UUID) error
	RefundBalance(ctx context.Context, userID uuid.UUID, amount int64, currency string, paymentID uuid.UUID, reason string) error
}

// GatewayService interfaz para interactuar con pasarelas de pago
type GatewayService interface {
	ProcessPayment(ctx context.Context, payment *Payment) (string, error)
}

// Service servicio principal de pagos
type Service struct {
	repo             Repository
	walletService    WalletService
	gatewayService   GatewayService
	eventPublisher   events.Publisher
	sagaOrchestrator *saga.Orchestrator
	logger           *zap.Logger
}

// NewService crea una nueva instancia del servicio de pagos
func NewService(
	repo Repository,
	walletService WalletService,
	gatewayService GatewayService,
	eventPublisher events.Publisher,
	logger *zap.Logger,
) *Service {
	return &Service{
		repo:             repo,
		walletService:    walletService,
		gatewayService:   gatewayService,
		eventPublisher:   eventPublisher,
		sagaOrchestrator: nil, // Se puede configurar después con SetSagaOrchestrator
		logger:           logger,
	}
}

// SetSagaOrchestrator configura el orquestador de sagas (opcional)
func (s *Service) SetSagaOrchestrator(orchestrator *saga.Orchestrator) {
	s.sagaOrchestrator = orchestrator
}

// ProcessPayment procesa una solicitud de pago
func (s *Service) ProcessPayment(ctx context.Context, req *PaymentRequest) (*Payment, error) {
	correlationID := uuid.New()
	paymentID := uuid.New()

	s.logger.Info("processing payment request",
		zap.String("payment_id", paymentID.String()),
		zap.String("user_id", req.UserID.String()),
		zap.Int64("amount", req.Amount),
		zap.String("correlation_id", correlationID.String()))

	// Crear entidad de pago
	payment := &Payment{
		ID:            paymentID,
		UserID:        req.UserID,
		Amount:        req.Amount,
		Currency:      req.Currency,
		ServiceID:     req.ServiceID,
		Description:   req.Description,
		Status:        StatusPending,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		CorrelationID: correlationID,
	}

	// Guardar pago inicial
	if err := s.repo.Save(ctx, payment); err != nil {
		s.logger.Error("failed to save payment", zap.Error(err))
		return nil, fmt.Errorf("failed to save payment: %w", err)
	}

	// Publicar evento de pago iniciado
	initiatedEvent := events.PaymentInitiatedEvent{
		BaseEvent: events.BaseEvent{
			ID:            uuid.New(),
			Type:          events.PaymentInitiated,
			AggregateID:   paymentID,
			Version:       1,
			Timestamp:     time.Now(),
			CorrelationID: correlationID,
		},
		UserID:      req.UserID,
		Amount:      req.Amount,
		Currency:    req.Currency,
		ServiceID:   req.ServiceID,
		Description: req.Description,
	}

	if err := s.eventPublisher.Publish(ctx, initiatedEvent); err != nil {
		s.logger.Error("failed to publish payment initiated event", zap.Error(err))
		// No retornamos error aquí, el pago puede continuar
	}

	// Iniciar Saga de pago - manejo automático de compensación
	if s.sagaOrchestrator != nil {
		saga, err := s.sagaOrchestrator.StartPaymentSaga(ctx, paymentID, req.UserID, req.Amount, req.Currency)
		if err != nil {
			s.logger.Error("failed to start payment saga",
				zap.String("payment_id", paymentID.String()),
				zap.Error(err))
			
			payment.Status = StatusFailed
			s.repo.UpdateStatus(ctx, paymentID, StatusFailed)
			s.publishPaymentFailed(ctx, payment, "saga_start_failed", err.Error())
			return payment, fmt.Errorf("failed to start payment saga: %w", err)
		}
		
		s.logger.Info("payment saga started",
			zap.String("payment_id", paymentID.String()),
			zap.String("saga_id", saga.ID.String()))
		
		// La saga maneja todo el flujo automáticamente
		payment.Status = StatusProcessing
		s.repo.UpdateStatus(ctx, paymentID, StatusProcessing)
		
		return payment, nil
	}

	// Fallback: Lógica manual (para compatibilidad)
	// Validar saldo en billetera
	if err := s.walletService.ValidateBalance(ctx, req.UserID, req.Amount, req.Currency); err != nil {
		s.logger.Error("insufficient balance",
			zap.String("payment_id", paymentID.String()),
			zap.Error(err))
		
		payment.Status = StatusFailed
		s.repo.UpdateStatus(ctx, paymentID, StatusFailed)
		
		// Publicar evento de pago fallido
		s.publishPaymentFailed(ctx, payment, "insufficient_balance", err.Error())
		return payment, fmt.Errorf("insufficient balance: %w", err)
	}

	// Deducir saldo de billetera
	if err := s.walletService.DeductBalance(ctx, req.UserID, req.Amount, req.Currency, paymentID); err != nil {
		s.logger.Error("failed to deduct balance",
			zap.String("payment_id", paymentID.String()),
			zap.Error(err))
		
		payment.Status = StatusFailed
		s.repo.UpdateStatus(ctx, paymentID, StatusFailed)
		
		s.publishPaymentFailed(ctx, payment, "deduction_failed", err.Error())
		return payment, fmt.Errorf("failed to deduct balance: %w", err)
	}

	// Actualizar estado a procesando
	payment.Status = StatusProcessing
	if err := s.repo.UpdateStatus(ctx, paymentID, StatusProcessing); err != nil {
		s.logger.Error("failed to update payment status", zap.Error(err))
	}

	// Procesar pago con pasarela externa (asíncrono)
	go func() {
		gatewayCtx := context.Background()
		// Create a copy of payment for goroutine to avoid modifying the returned object
		paymentCopy := *payment
		gatewayTxnID, err := s.gatewayService.ProcessPayment(gatewayCtx, &paymentCopy)
		
		if err != nil {
			s.logger.Error("gateway processing failed",
				zap.String("payment_id", paymentID.String()),
				zap.Error(err))
			
			// Reembolsar saldo
			s.walletService.RefundBalance(gatewayCtx, paymentCopy.UserID, paymentCopy.Amount, paymentCopy.Currency, paymentID, "gateway_failure")
			
			// Actualizar estado y publicar evento de falla
			s.repo.UpdateStatus(gatewayCtx, paymentID, StatusFailed)
			s.publishPaymentFailed(gatewayCtx, &paymentCopy, "gateway_error", err.Error())
			return
		}

		// Pago exitoso
		paymentCopy.GatewayTxnID = gatewayTxnID
		paymentCopy.Status = StatusCompleted
		s.repo.UpdateStatus(gatewayCtx, paymentID, StatusCompleted)
		
		// Publicar evento de pago completado
		completedEvent := events.PaymentCompletedEvent{
			BaseEvent: events.BaseEvent{
				ID:            uuid.New(),
				Type:          events.PaymentCompleted,
				AggregateID:   paymentID,
				Version:       1,
				Timestamp:     time.Now(),
				CorrelationID: correlationID,
			},
			UserID:        paymentCopy.UserID,
			Amount:        paymentCopy.Amount,
			Currency:      paymentCopy.Currency,
			GatewayTxnID:  gatewayTxnID,
		}

		s.eventPublisher.Publish(gatewayCtx, completedEvent)
		
		s.logger.Info("payment completed successfully",
			zap.String("payment_id", paymentID.String()),
			zap.String("gateway_txn_id", gatewayTxnID))
	}()

	return payment, nil
}

// GetPayment obtiene un pago por ID
func (s *Service) GetPayment(ctx context.Context, id uuid.UUID) (*Payment, error) {
	return s.repo.GetByID(ctx, id)
}

// publishPaymentFailed publica evento de pago fallido
func (s *Service) publishPaymentFailed(ctx context.Context, payment *Payment, errorCode, reason string) {
	failedEvent := events.PaymentFailedEvent{
		BaseEvent: events.BaseEvent{
			ID:            uuid.New(),
			Type:          events.PaymentFailed,
			AggregateID:   payment.ID,
			Version:       2,
			Timestamp:     time.Now(),
			CorrelationID: payment.CorrelationID,
		},
		UserID:    payment.UserID,
		Amount:    payment.Amount,
		Currency:  payment.Currency,
		Reason:    reason,
		ErrorCode: errorCode,
	}

	if err := s.eventPublisher.Publish(ctx, failedEvent); err != nil {
		s.logger.Error("failed to publish payment failed event", zap.Error(err))
	}
}
