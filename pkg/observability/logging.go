package observability

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// StructuredLogger define la interfaz para logging estructurado
type StructuredLogger interface {
	Info(msg string, fields ...zap.Field)
	Warn(msg string, fields ...zap.Field)
	Error(msg string, fields ...zap.Field)
	Debug(msg string, fields ...zap.Field)
	Fatal(msg string, fields ...zap.Field)
	With(fields ...zap.Field) StructuredLogger
	WithContext(ctx context.Context) StructuredLogger
}

// ZapLogger implementa StructuredLogger usando uber-go/zap
type ZapLogger struct {
	logger *zap.Logger
}

// NewZapLogger crea un nuevo logger estructurado con zap
func NewZapLogger(level string, isDevelopment bool) (*ZapLogger, error) {
	var config zap.Config
	
	if isDevelopment {
		config = zap.NewDevelopmentConfig()
		config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	} else {
		config = zap.NewProductionConfig()
		config.EncoderConfig.TimeKey = "timestamp"
		config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	}
	
	// Configurar nivel de logging
	switch level {
	case "debug":
		config.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	case "info":
		config.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	case "warn":
		config.Level = zap.NewAtomicLevelAt(zap.WarnLevel)
	case "error":
		config.Level = zap.NewAtomicLevelAt(zap.ErrorLevel)
	default:
		config.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	}
	
	logger, err := config.Build(
		zap.AddCallerSkip(1), // Skip wrapper functions
		zap.AddStacktrace(zapcore.ErrorLevel),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create zap logger: %w", err)
	}
	
	return &ZapLogger{logger: logger}, nil
}

// Info logs an info message with structured fields
func (z *ZapLogger) Info(msg string, fields ...zap.Field) {
	z.logger.Info(msg, fields...)
}

// Warn logs a warning message with structured fields
func (z *ZapLogger) Warn(msg string, fields ...zap.Field) {
	z.logger.Warn(msg, fields...)
}

// Error logs an error message with structured fields
func (z *ZapLogger) Error(msg string, fields ...zap.Field) {
	z.logger.Error(msg, fields...)
}

// Debug logs a debug message with structured fields
func (z *ZapLogger) Debug(msg string, fields ...zap.Field) {
	z.logger.Debug(msg, fields...)
}

// Fatal logs a fatal message and exits
func (z *ZapLogger) Fatal(msg string, fields ...zap.Field) {
	z.logger.Fatal(msg, fields...)
}

// With creates a child logger with additional fields
func (z *ZapLogger) With(fields ...zap.Field) StructuredLogger {
	return &ZapLogger{logger: z.logger.With(fields...)}
}

// WithContext creates a logger with context information
func (z *ZapLogger) WithContext(ctx context.Context) StructuredLogger {
	fields := extractFieldsFromContext(ctx)
	return z.With(fields...)
}

// Sync flushes any buffered log entries
func (z *ZapLogger) Sync() error {
	return z.logger.Sync()
}

// extractFieldsFromContext extrae campos del contexto para logging
func extractFieldsFromContext(ctx context.Context) []zap.Field {
	var fields []zap.Field
	
	// Extraer trace ID si existe
	if traceID := ctx.Value("trace_id"); traceID != nil {
		fields = append(fields, zap.String("trace_id", traceID.(string)))
	}
	
	// Extraer correlation ID si existe
	if correlationID := ctx.Value("correlation_id"); correlationID != nil {
		fields = append(fields, zap.String("correlation_id", correlationID.(string)))
	}
	
	// Extraer user ID si existe
	if userID := ctx.Value("user_id"); userID != nil {
		fields = append(fields, zap.String("user_id", userID.(string)))
	}
	
	// Extraer request ID si existe
	if requestID := ctx.Value("request_id"); requestID != nil {
		fields = append(fields, zap.String("request_id", requestID.(string)))
	}
	
	return fields
}

// PaymentLogger proporciona logging específico para operaciones de pago
type PaymentLogger struct {
	logger StructuredLogger
}

// NewPaymentLogger crea un logger específico para pagos
func NewPaymentLogger(logger StructuredLogger) *PaymentLogger {
	return &PaymentLogger{
		logger: logger.With(zap.String("service", "payment")),
	}
}

// LogPaymentStarted registra el inicio de un pago
func (pl *PaymentLogger) LogPaymentStarted(ctx context.Context, paymentID, userID string, amount float64, currency string) {
	pl.logger.WithContext(ctx).Info("Payment processing started",
		zap.String("payment_id", paymentID),
		zap.String("user_id", userID),
		zap.Float64("amount", amount),
		zap.String("currency", currency),
		zap.String("event", "payment_started"),
		zap.Time("timestamp", time.Now().UTC()),
	)
}

// LogPaymentCompleted registra la finalización exitosa de un pago
func (pl *PaymentLogger) LogPaymentCompleted(ctx context.Context, paymentID, userID string, amount float64, currency, gatewayTxID string, processingTimeMs int64) {
	pl.logger.WithContext(ctx).Info("Payment completed successfully",
		zap.String("payment_id", paymentID),
		zap.String("user_id", userID),
		zap.Float64("amount", amount),
		zap.String("currency", currency),
		zap.String("gateway_transaction_id", gatewayTxID),
		zap.Int64("processing_time_ms", processingTimeMs),
		zap.String("event", "payment_completed"),
		zap.String("status", "success"),
		zap.Time("timestamp", time.Now().UTC()),
	)
}

// LogPaymentFailed registra el fallo de un pago
func (pl *PaymentLogger) LogPaymentFailed(ctx context.Context, paymentID, userID string, amount float64, currency, failureReason string, err error) {
	fields := []zap.Field{
		zap.String("payment_id", paymentID),
		zap.String("user_id", userID),
		zap.Float64("amount", amount),
		zap.String("currency", currency),
		zap.String("failure_reason", failureReason),
		zap.String("event", "payment_failed"),
		zap.String("status", "failed"),
		zap.Time("timestamp", time.Now().UTC()),
	}
	
	if err != nil {
		fields = append(fields, zap.Error(err))
	}
	
	pl.logger.WithContext(ctx).Error("Payment failed", fields...)
}

// LogGatewayError registra errores específicos de la pasarela
func (pl *PaymentLogger) LogGatewayError(ctx context.Context, paymentID, gatewayProvider, errorCode, errorMessage string) {
	pl.logger.WithContext(ctx).Error("Gateway error occurred",
		zap.String("payment_id", paymentID),
		zap.String("gateway_provider", gatewayProvider),
		zap.String("error_code", errorCode),
		zap.String("error_message", errorMessage),
		zap.String("event", "gateway_error"),
		zap.String("component", "external_gateway"),
		zap.Time("timestamp", time.Now().UTC()),
	)
}

// LogRetryAttempt registra intentos de reintento
func (pl *PaymentLogger) LogRetryAttempt(ctx context.Context, paymentID string, attempt, maxAttempts int, nextRetryIn time.Duration, lastError string) {
	pl.logger.WithContext(ctx).Warn("Payment retry attempt",
		zap.String("payment_id", paymentID),
		zap.Int("attempt", attempt),
		zap.Int("max_attempts", maxAttempts),
		zap.Duration("next_retry_in", nextRetryIn),
		zap.String("last_error", lastError),
		zap.String("event", "payment_retry"),
		zap.Time("timestamp", time.Now().UTC()),
	)
}

// WalletLogger proporciona logging específico para operaciones de billetera
type WalletLogger struct {
	logger StructuredLogger
}

// NewWalletLogger crea un logger específico para billeteras
func NewWalletLogger(logger StructuredLogger) *WalletLogger {
	return &WalletLogger{
		logger: logger.With(zap.String("service", "wallet")),
	}
}

// LogFundsDeducted registra deducción de fondos
func (wl *WalletLogger) LogFundsDeducted(ctx context.Context, userID, currency string, amount, previousBalance, newBalance float64, reason string) {
	wl.logger.WithContext(ctx).Info("Funds deducted from wallet",
		zap.String("user_id", userID),
		zap.String("currency", currency),
		zap.Float64("amount", amount),
		zap.Float64("previous_balance", previousBalance),
		zap.Float64("new_balance", newBalance),
		zap.String("reason", reason),
		zap.String("event", "funds_deducted"),
		zap.String("operation", "debit"),
		zap.Time("timestamp", time.Now().UTC()),
	)
}

// LogFundsAdded registra adición de fondos
func (wl *WalletLogger) LogFundsAdded(ctx context.Context, userID, currency string, amount, previousBalance, newBalance float64, reason string) {
	wl.logger.WithContext(ctx).Info("Funds added to wallet",
		zap.String("user_id", userID),
		zap.String("currency", currency),
		zap.Float64("amount", amount),
		zap.Float64("previous_balance", previousBalance),
		zap.Float64("new_balance", newBalance),
		zap.String("reason", reason),
		zap.String("event", "funds_added"),
		zap.String("operation", "credit"),
		zap.Time("timestamp", time.Now().UTC()),
	)
}

// LogInsufficientFunds registra intentos con saldo insuficiente
func (wl *WalletLogger) LogInsufficientFunds(ctx context.Context, userID, currency string, requestedAmount, availableBalance float64) {
	wl.logger.WithContext(ctx).Warn("Insufficient funds for operation",
		zap.String("user_id", userID),
		zap.String("currency", currency),
		zap.Float64("requested_amount", requestedAmount),
		zap.Float64("available_balance", availableBalance),
		zap.Float64("shortfall", requestedAmount-availableBalance),
		zap.String("event", "insufficient_funds"),
		zap.String("status", "rejected"),
		zap.Time("timestamp", time.Now().UTC()),
	)
}

// LogConcurrencyConflict registra conflictos de concurrencia
func (wl *WalletLogger) LogConcurrencyConflict(ctx context.Context, userID, currency string, expectedVersion, actualVersion int64, attempt int) {
	wl.logger.WithContext(ctx).Warn("Concurrency conflict detected",
		zap.String("user_id", userID),
		zap.String("currency", currency),
		zap.Int64("expected_version", expectedVersion),
		zap.Int64("actual_version", actualVersion),
		zap.Int("retry_attempt", attempt),
		zap.String("event", "concurrency_conflict"),
		zap.String("strategy", "optimistic_locking"),
		zap.Time("timestamp", time.Now().UTC()),
	)
}

// EventLogger proporciona logging específico para eventos
type EventLogger struct {
	logger StructuredLogger
}

// NewEventLogger crea un logger específico para eventos
func NewEventLogger(logger StructuredLogger) *EventLogger {
	return &EventLogger{
		logger: logger.With(zap.String("service", "eventstore")),
	}
}

// LogEventStored registra almacenamiento de eventos
func (el *EventLogger) LogEventStored(ctx context.Context, eventID, eventType, aggregateID, aggregateType string, sequenceNumber int64) {
	el.logger.WithContext(ctx).Debug("Event stored successfully",
		zap.String("event_id", eventID),
		zap.String("event_type", eventType),
		zap.String("aggregate_id", aggregateID),
		zap.String("aggregate_type", aggregateType),
		zap.Int64("sequence_number", sequenceNumber),
		zap.String("event", "event_stored"),
		zap.Time("timestamp", time.Now().UTC()),
	)
}

// LogEventPublished registra publicación de eventos
func (el *EventLogger) LogEventPublished(ctx context.Context, eventID, eventType string, publishLatencyMs int64) {
	el.logger.WithContext(ctx).Debug("Event published to event bus",
		zap.String("event_id", eventID),
		zap.String("event_type", eventType),
		zap.Int64("publish_latency_ms", publishLatencyMs),
		zap.String("event", "event_published"),
		zap.Time("timestamp", time.Now().UTC()),
	)
}

// LogEventProcessingError registra errores de procesamiento de eventos
func (el *EventLogger) LogEventProcessingError(ctx context.Context, eventID, eventType, processorName string, err error, attempt int) {
	el.logger.WithContext(ctx).Error("Event processing failed",
		zap.String("event_id", eventID),
		zap.String("event_type", eventType),
		zap.String("processor", processorName),
		zap.Error(err),
		zap.Int("attempt", attempt),
		zap.String("event", "event_processing_error"),
		zap.Time("timestamp", time.Now().UTC()),
	)
}

// LogEventSentToDLQ registra envío de eventos a DLQ
func (el *EventLogger) LogEventSentToDLQ(ctx context.Context, eventID, eventType, reason string, failureCount int) {
	el.logger.WithContext(ctx).Error("Event sent to Dead Letter Queue",
		zap.String("event_id", eventID),
		zap.String("event_type", eventType),
		zap.String("failure_reason", reason),
		zap.Int("failure_count", failureCount),
		zap.String("event", "event_sent_to_dlq"),
		zap.String("status", "failed_permanently"),
		zap.Time("timestamp", time.Now().UTC()),
	)
}

// SagaLogger proporciona logging específico para sagas
type SagaLogger struct {
	logger StructuredLogger
}

// NewSagaLogger crea un logger específico para sagas
func NewSagaLogger(logger StructuredLogger) *SagaLogger {
	return &SagaLogger{
		logger: logger.With(zap.String("service", "saga_orchestrator")),
	}
}

// LogSagaStarted registra inicio de saga
func (sl *SagaLogger) LogSagaStarted(ctx context.Context, sagaID, sagaType, paymentID string) {
	sl.logger.WithContext(ctx).Info("Saga started",
		zap.String("saga_id", sagaID),
		zap.String("saga_type", sagaType),
		zap.String("payment_id", paymentID),
		zap.String("event", "saga_started"),
		zap.String("status", "initiated"),
		zap.Time("timestamp", time.Now().UTC()),
	)
}

// LogSagaStepCompleted registra finalización de paso de saga
func (sl *SagaLogger) LogSagaStepCompleted(ctx context.Context, sagaID, stepName string, stepDuration time.Duration) {
	sl.logger.WithContext(ctx).Info("Saga step completed",
		zap.String("saga_id", sagaID),
		zap.String("step_name", stepName),
		zap.Duration("step_duration", stepDuration),
		zap.String("event", "saga_step_completed"),
		zap.String("status", "success"),
		zap.Time("timestamp", time.Now().UTC()),
	)
}

// LogSagaCompensationStarted registra inicio de compensación
func (sl *SagaLogger) LogSagaCompensationStarted(ctx context.Context, sagaID, reason string, completedSteps []string) {
	sl.logger.WithContext(ctx).Warn("Saga compensation started",
		zap.String("saga_id", sagaID),
		zap.String("compensation_reason", reason),
		zap.Strings("completed_steps", completedSteps),
		zap.String("event", "saga_compensation_started"),
		zap.String("status", "compensating"),
		zap.Time("timestamp", time.Now().UTC()),
	)
}

// LogSagaCompleted registra finalización exitosa de saga
func (sl *SagaLogger) LogSagaCompleted(ctx context.Context, sagaID string, totalDuration time.Duration, completedSteps []string) {
	sl.logger.WithContext(ctx).Info("Saga completed successfully",
		zap.String("saga_id", sagaID),
		zap.Duration("total_duration", totalDuration),
		zap.Strings("completed_steps", completedSteps),
		zap.String("event", "saga_completed"),
		zap.String("status", "success"),
		zap.Time("timestamp", time.Now().UTC()),
	)
}

// LogSagaFailed registra fallo de saga
func (sl *SagaLogger) LogSagaFailed(ctx context.Context, sagaID, failureReason string, err error, compensationSteps []string) {
	fields := []zap.Field{
		zap.String("saga_id", sagaID),
		zap.String("failure_reason", failureReason),
		zap.Strings("compensation_steps", compensationSteps),
		zap.String("event", "saga_failed"),
		zap.String("status", "failed"),
		zap.Time("timestamp", time.Now().UTC()),
	}
	
	if err != nil {
		fields = append(fields, zap.Error(err))
	}
	
	sl.logger.WithContext(ctx).Error("Saga failed", fields...)
}

// LoggingMiddleware proporciona middleware para logging de HTTP requests
func LoggingMiddleware(logger StructuredLogger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			
			// Crear contexto con información de request
			ctx := r.Context()
			requestID := uuid.New().String()
			ctx = context.WithValue(ctx, "request_id", requestID)
			r = r.WithContext(ctx)
			
			// Wrapper para capturar status code
			wrapped := &responseWriter{ResponseWriter: w, statusCode: 200}
			
			// Log request
			logger.WithContext(ctx).Info("HTTP request started",
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.String("remote_addr", r.RemoteAddr),
				zap.String("user_agent", r.UserAgent()),
				zap.String("request_id", requestID),
			)
			
			// Procesar request
			next.ServeHTTP(wrapped, r)
			
			// Log response
			duration := time.Since(start)
			logger.WithContext(ctx).Info("HTTP request completed",
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.Int("status_code", wrapped.statusCode),
				zap.Duration("duration", duration),
				zap.String("request_id", requestID),
			)
		})
	}
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// ContextWithTracing agrega información de tracing al contexto
func ContextWithTracing(ctx context.Context, traceID, correlationID, userID string) context.Context {
	ctx = context.WithValue(ctx, "trace_id", traceID)
	ctx = context.WithValue(ctx, "correlation_id", correlationID)
	if userID != "" {
		ctx = context.WithValue(ctx, "user_id", userID)
	}
	return ctx
}
