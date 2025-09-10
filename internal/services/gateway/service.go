package gateway

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"time"

	"payment-system/internal/events"
	"payment-system/internal/services/payment"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// GatewayProvider representa diferentes proveedores de pasarela
type GatewayProvider string

const (
	ProviderStripe      GatewayProvider = "stripe"
	ProviderPayPal      GatewayProvider = "paypal"
	ProviderMercadoPago GatewayProvider = "mercadopago"
)

// GatewayRequest estructura para solicitudes a pasarela externa
type GatewayRequest struct {
	PaymentID   uuid.UUID              `json:"payment_id"`
	Amount      int64                  `json:"amount"`
	Currency    string                 `json:"currency"`
	UserID      uuid.UUID              `json:"user_id"`
	Description string                 `json:"description"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// GatewayResponse estructura para respuestas de pasarela externa
type GatewayResponse struct {
	TransactionID string                 `json:"transaction_id"`
	Status        string                 `json:"status"`
	ResponseCode  string                 `json:"response_code"`
	Message       string                 `json:"message"`
	ProcessedAt   time.Time              `json:"processed_at"`
	Data          map[string]interface{} `json:"data"`
}

// CircuitBreakerState estados del circuit breaker
type CircuitBreakerState int

const (
	StateClosed CircuitBreakerState = iota
	StateOpen
	StateHalfOpen
)

// CircuitBreaker implementa patrón circuit breaker para resiliencia
type CircuitBreaker struct {
	maxFailures     int
	resetTimeout    time.Duration
	currentFailures int
	lastFailureTime time.Time
	state           CircuitBreakerState
}

// NewCircuitBreaker crea un nuevo circuit breaker
func NewCircuitBreaker(maxFailures int, resetTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		maxFailures:  maxFailures,
		resetTimeout: resetTimeout,
		state:        StateClosed,
	}
}

// CanExecute verifica si se puede ejecutar una operación
func (cb *CircuitBreaker) CanExecute() bool {
	switch cb.state {
	case StateClosed:
		return true
	case StateOpen:
		if time.Since(cb.lastFailureTime) > cb.resetTimeout {
			cb.state = StateHalfOpen
			return true
		}
		return false
	case StateHalfOpen:
		return true
	default:
		return false
	}
}

// OnSuccess registra una ejecución exitosa
func (cb *CircuitBreaker) OnSuccess() {
	cb.currentFailures = 0
	cb.state = StateClosed
}

// OnFailure registra una falla
func (cb *CircuitBreaker) OnFailure() {
	cb.currentFailures++
	cb.lastFailureTime = time.Now()

	if cb.currentFailures >= cb.maxFailures {
		cb.state = StateOpen
	}
}

// Service servicio de pasarela de pagos
type Service struct {
	httpClient     *http.Client
	eventPublisher events.Publisher
	circuitBreaker *CircuitBreaker
	logger         *zap.Logger
	baseURL        string
	apiKey         string
	provider       GatewayProvider
}

// NewService crea una nueva instancia del servicio de pasarela
func NewService(
	baseURL, apiKey string,
	provider GatewayProvider,
	eventPublisher events.Publisher,
	logger *zap.Logger,
) *Service {
	return &Service{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		eventPublisher: eventPublisher,
		circuitBreaker: NewCircuitBreaker(5, 60*time.Second),
		logger:         logger,
		baseURL:        baseURL,
		apiKey:         apiKey,
		provider:       provider,
	}
}

// ProcessPayment procesa un pago con la pasarela externa
func (s *Service) ProcessPayment(ctx context.Context, payment *payment.Payment) (string, error) {
	startTime := time.Now()

	s.logger.Info("processing payment with gateway",
		zap.String("payment_id", payment.ID.String()),
		zap.String("provider", string(s.provider)),
		zap.Int64("amount", payment.Amount))

	// Verificar circuit breaker
	if !s.circuitBreaker.CanExecute() {
		err := fmt.Errorf("circuit breaker is open")
		s.logger.Error("circuit breaker prevented execution", zap.Error(err))

		// Publicar evento de error
		s.publishGatewayError(ctx, payment, "circuit_breaker_open", err.Error())
		return "", err
	}

	// Publicar evento de solicitud enviada
	s.publishGatewayRequestSent(ctx, payment)

	// Preparar solicitud
	gatewayReq := &GatewayRequest{
		PaymentID:   payment.ID,
		Amount:      payment.Amount,
		Currency:    payment.Currency,
		UserID:      payment.UserID,
		Description: payment.Description,
		Metadata: map[string]interface{}{
			"correlation_id": payment.CorrelationID.String(),
			"service_id":     payment.ServiceID,
		},
	}

	// Simular procesamiento con pasarela externa
	response, err := s.callGatewayAPI(ctx, gatewayReq)
	processingTime := time.Since(startTime)

	if err != nil {
		s.circuitBreaker.OnFailure()
		s.logger.Error("gateway API call failed",
			zap.String("payment_id", payment.ID.String()),
			zap.Duration("processing_time", processingTime),
			zap.Error(err))

		// Publicar evento de error
		s.publishGatewayError(ctx, payment, "api_call_failed", err.Error())
		return "", fmt.Errorf("gateway processing failed: %w", err)
	}

	s.circuitBreaker.OnSuccess()

	// Publicar evento de respuesta recibida
	s.publishGatewayResponseReceived(ctx, payment, response, processingTime)

	// Verificar si el pago fue exitoso
	if response.Status != "success" && response.Status != "completed" {
		err := fmt.Errorf("payment rejected by gateway: %s", response.Message)
		s.logger.Error("payment rejected by gateway",
			zap.String("payment_id", payment.ID.String()),
			zap.String("status", response.Status),
			zap.String("message", response.Message))
		return "", err
	}

	s.logger.Info("payment processed successfully by gateway",
		zap.String("payment_id", payment.ID.String()),
		zap.String("gateway_txn_id", response.TransactionID),
		zap.Duration("processing_time", processingTime))

	return response.TransactionID, nil
}

// callGatewayAPI simula llamada a API de pasarela externa
func (s *Service) callGatewayAPI(ctx context.Context, req *GatewayRequest) (*GatewayResponse, error) {
	// En una implementación real, aquí haríamos la llamada HTTP a la pasarela
	// Por ahora, simulamos el comportamiento

	// Simular latencia de red
	select {
	case <-time.After(time.Duration(100+rand.Intn(400)) * time.Millisecond):
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Simular diferentes escenarios
	scenario := rand.Intn(100)

	switch {
	case scenario < 85: // 85% éxito
		return &GatewayResponse{
			TransactionID: fmt.Sprintf("gw_%s_%d", s.provider, time.Now().Unix()),
			Status:        "success",
			ResponseCode:  "00",
			Message:       "Payment processed successfully",
			ProcessedAt:   time.Now(),
			Data: map[string]interface{}{
				"provider":   string(s.provider),
				"payment_id": req.PaymentID.String(),
				"amount":     req.Amount,
				"currency":   req.Currency,
			},
		}, nil
	case scenario < 95: // 10% rechazo
		return &GatewayResponse{
			TransactionID: "",
			Status:        "declined",
			ResponseCode:  "05",
			Message:       "Payment declined by issuer",
			ProcessedAt:   time.Now(),
			Data:          map[string]interface{}{},
		}, nil
	default: // 5% error técnico
		return nil, fmt.Errorf("gateway service temporarily unavailable")
	}
}

// publishGatewayRequestSent publica evento de solicitud enviada
func (s *Service) publishGatewayRequestSent(ctx context.Context, payment *payment.Payment) {
	event := events.BaseEvent{
		ID:            uuid.New(),
		Type:          events.GatewayRequestSent,
		AggregateID:   payment.ID,
		Version:       1,
		Timestamp:     time.Now(),
		CorrelationID: payment.CorrelationID,
		Metadata: map[string]interface{}{
			"provider": string(s.provider),
			"amount":   payment.Amount,
			"currency": payment.Currency,
			"user_id":  payment.UserID.String(),
		},
	}

	if err := s.eventPublisher.Publish(ctx, event); err != nil {
		s.logger.Error("failed to publish gateway request sent event", zap.Error(err))
	}
}

// publishGatewayResponseReceived publica evento de respuesta recibida
func (s *Service) publishGatewayResponseReceived(ctx context.Context, payment *payment.Payment, response *GatewayResponse, processingTime time.Duration) {
	event := events.GatewayResponseReceivedEvent{
		BaseEvent: events.BaseEvent{
			ID:            uuid.New(),
			Type:          events.GatewayResponseReceived,
			AggregateID:   payment.ID,
			Version:       2,
			Timestamp:     time.Now(),
			CorrelationID: payment.CorrelationID,
		},
		PaymentID:      payment.ID,
		GatewayTxnID:   response.TransactionID,
		Status:         response.Status,
		ResponseCode:   response.ResponseCode,
		ResponseData:   response.Data,
		ProcessingTime: processingTime,
	}

	if err := s.eventPublisher.Publish(ctx, event); err != nil {
		s.logger.Error("failed to publish gateway response received event", zap.Error(err))
	}
}

// publishGatewayError publica evento de error de pasarela
func (s *Service) publishGatewayError(ctx context.Context, payment *payment.Payment, errorCode, message string) {
	event := events.BaseEvent{
		ID:            uuid.New(),
		Type:          events.GatewayError,
		AggregateID:   payment.ID,
		Version:       2,
		Timestamp:     time.Now(),
		CorrelationID: payment.CorrelationID,
		Metadata: map[string]interface{}{
			"error_code": errorCode,
			"message":    message,
			"provider":   string(s.provider),
		},
	}

	if err := s.eventPublisher.Publish(ctx, event); err != nil {
		s.logger.Error("failed to publish gateway error event", zap.Error(err))
	}
}

// GetCircuitBreakerState obtiene el estado actual del circuit breaker
func (s *Service) GetCircuitBreakerState() CircuitBreakerState {
	return s.circuitBreaker.state
}

// ResetCircuitBreaker resetea manualmente el circuit breaker
func (s *Service) ResetCircuitBreaker() {
	s.circuitBreaker.state = StateClosed
	s.circuitBreaker.currentFailures = 0
}
