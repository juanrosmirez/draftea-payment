package observability

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

// ObservabilityExample demuestra la integración completa de observabilidad
type ObservabilityExample struct {
	logger        StructuredLogger
	metrics       MetricsCollector
	healthChecker *HealthChecker
	healthHandler *HealthHandler
}

// NewObservabilityExample crea un ejemplo completo de observabilidad
func NewObservabilityExample(
	db *sql.DB,
	eventStore EventStoreInterface,
	redisClient RedisClient,
) (*ObservabilityExample, error) {
	// Configurar logger estructurado
	logger, err := NewZapLogger("info", false)
	if err != nil {
		return nil, fmt.Errorf("failed to create logger: %w", err)
	}

	// Crear collector de métricas
	metrics := NewMetricsCollector("payment_system")
	
	// Configurar health checks
	healthChecker := SetupHealthChecks(db, eventStore, redisClient, logger)
	healthHandler := NewHealthHandler(healthChecker, logger)

	return &ObservabilityExample{
		logger:        logger,
		metrics:       metrics,
		healthChecker: healthChecker,
		healthHandler: healthHandler,
	}, nil
}

// StartHTTPServer inicia el servidor HTTP con endpoints de observabilidad
func (oe *ObservabilityExample) StartHTTPServer(port string) error {
	mux := http.NewServeMux()

	// Endpoints de health checks
	mux.HandleFunc("/health", oe.healthHandler.HealthzHandler)
	mux.HandleFunc("/healthz", oe.healthHandler.HealthzHandler)
	mux.HandleFunc("/ready", oe.healthHandler.ReadyHandler)
	mux.HandleFunc("/live", oe.healthHandler.LiveHandler)

	// Endpoint de métricas Prometheus
	mux.Handle("/metrics", promhttp.Handler())

	// Middleware de logging y métricas para todas las rutas
	handler := oe.withObservabilityMiddleware(mux)

	server := &http.Server{
		Addr:    ":" + port,
		Handler: handler,
	}

	oe.logger.Info("Starting HTTP server with observability endpoints",
		zap.String("port", port),
		zap.Strings("endpoints", []string{"/health", "/ready", "/live", "/metrics"}),
	)

	return server.ListenAndServe()
}

// withObservabilityMiddleware añade middleware de logging y métricas
func (oe *ObservabilityExample) withObservabilityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		
		// Crear contexto con trace ID
		ctx := WithTraceID(r.Context(), GenerateTraceID())
		r = r.WithContext(ctx)

		// Wrapper para capturar status code
		wrapper := &responseWrapper{ResponseWriter: w, statusCode: 200}

		// Log request
		oe.logger.WithContext(ctx).Info("HTTP request started",
			zap.String("method", r.Method),
			zap.String("path", r.URL.Path),
			zap.String("remote_addr", r.RemoteAddr),
			zap.String("user_agent", r.UserAgent()),
		)

		// Ejecutar handler
		next.ServeHTTP(wrapper, r)

		// Calcular duración
		duration := time.Since(start)

		// Actualizar métricas
		oe.metrics.IncrementHTTPRequestCounter(r.Method, r.URL.Path, wrapper.statusCode)
		oe.metrics.RecordHTTPRequestDuration(r.Method, r.URL.Path, wrapper.statusCode, duration)

		// Log response
		oe.logger.WithContext(ctx).Info("HTTP request completed",
			zap.Int("status_code", wrapper.statusCode),
			zap.Duration("duration", duration),
		)
	})
}

// responseWrapper captura el status code de la respuesta
type responseWrapper struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWrapper) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// DemoPaymentProcessingWithObservability demuestra el procesamiento de pagos con observabilidad completa
func (oe *ObservabilityExample) DemoPaymentProcessingWithObservability() {
	ctx := context.Background()
	
	// Simular procesamiento de pagos con observabilidad
	oe.logger.Info("Starting payment processing demo")

	// Simular diferentes tipos de pagos
	paymentScenarios := []struct {
		paymentID string
		userID    string
		amount    float64
		success   bool
	}{
		{"pay_001", "user_123", 100.50, true},
		{"pay_002", "user_456", 250.00, true},
		{"pay_003", "user_789", 50.25, false}, // Fallo simulado
		{"pay_004", "user_123", 75.00, true},
	}

	for _, scenario := range paymentScenarios {
		oe.processPaymentWithObservability(ctx, scenario.paymentID, scenario.userID, scenario.amount, scenario.success)
		time.Sleep(100 * time.Millisecond) // Pequeña pausa entre pagos
	}

	oe.logger.Info("Payment processing demo completed")
}

// processPaymentWithObservability procesa un pago con observabilidad completa
func (oe *ObservabilityExample) processPaymentWithObservability(
	ctx context.Context,
	paymentID, userID string,
	amount float64,
	shouldSucceed bool,
) {
	start := time.Now()
	
	// Crear contexto con información del pago
	ctx = WithPaymentID(ctx, paymentID)
	ctx = WithUserID(ctx, userID)
	ctx = WithTraceID(ctx, GenerateTraceID())

	logger := oe.logger.WithContext(ctx)
	
	logger.Info("Processing payment",
		zap.String("payment_id", paymentID),
		zap.String("user_id", userID),
		zap.Float64("amount", amount),
	)

	// Incrementar contador de intentos de pago
	oe.metrics.IncrementPaymentCounter("attempted", "USD", "credit_card")

	// Simular validación de pago
	if amount <= 0 {
		logger.Error("Invalid payment amount",
			zap.Float64("amount", amount),
		)
		oe.metrics.IncrementPaymentCounter("failed", "USD", "credit_card")
		oe.metrics.IncrementErrorCounter("payment", "validation_error")
		return
	}

	// Simular procesamiento
	time.Sleep(50 * time.Millisecond)

	if shouldSucceed {
		// Pago exitoso
		duration := time.Since(start)
		
		logger.Info("Payment processed successfully",
			zap.Duration("processing_time", duration),
		)

		// Actualizar métricas de éxito
		oe.metrics.IncrementPaymentCounter("success", "USD", "credit_card")
		oe.metrics.RecordPaymentLatency(duration, "success", "USD")

		// Simular actualización de billetera
		oe.updateWalletWithObservability(ctx, userID, amount, "debit")

	} else {
		// Pago fallido
		duration := time.Since(start)
		
		logger.Error("Payment processing failed",
			zap.String("error", "insufficient_funds"),
			zap.Duration("processing_time", duration),
		)

		// Actualizar métricas de fallo
		oe.metrics.IncrementPaymentCounter("failed", "USD", "credit_card")
		oe.metrics.RecordPaymentLatency(duration, "failed", "USD")
		oe.metrics.IncrementErrorCounter("payment", "insufficient_funds")

		// Simular reintento
		oe.simulateRetryWithObservability(ctx, paymentID, 1)
	}
}

// updateWalletWithObservability actualiza billetera con observabilidad
func (oe *ObservabilityExample) updateWalletWithObservability(
	ctx context.Context,
	userID string,
	amount float64,
	operation string,
) {
	start := time.Now()
	
	logger := oe.logger.WithContext(ctx)
	
	logger.Info("Updating wallet",
		zap.String("user_id", userID),
		zap.Float64("amount", amount),
		zap.String("operation", operation),
	)

	// Simular operación de billetera
	time.Sleep(20 * time.Millisecond)

	duration := time.Since(start)

	// Actualizar métricas de billetera
	oe.metrics.IncrementWalletOperationCounter(operation, "USD")

	// Simular balance actualizado
	newBalance := 500.0 - amount // Balance simulado
	oe.metrics.RecordWalletBalance(userID, "USD", newBalance)

	logger.Info("Wallet updated successfully",
		zap.Float64("new_balance", newBalance),
		zap.Duration("operation_time", duration),
	)
}

// simulateRetryWithObservability simula reintentos con observabilidad
func (oe *ObservabilityExample) simulateRetryWithObservability(
	ctx context.Context,
	paymentID string,
	attempt int,
) {
	logger := oe.logger.WithContext(ctx)
	
	logger.Warn("Retrying payment",
		zap.String("payment_id", paymentID),
		zap.Int("attempt", attempt),
	)

	// Actualizar métricas de reintento
	oe.metrics.IncrementRetryCounter("payment", "processing")

	// Simular reintento
	time.Sleep(100 * time.Millisecond)

	if attempt < 3 {
		// Simular otro fallo y reintento
		oe.simulateRetryWithObservability(ctx, paymentID, attempt+1)
	} else {
		// Enviar a DLQ después de max reintentos
		logger.Error("Payment failed after max retries, sending to DLQ",
			zap.String("payment_id", paymentID),
			zap.Int("max_attempts", attempt),
		)
		oe.metrics.IncrementDLQCounter("payment_processing")
	}
}

// DemoHealthChecks ejecuta y muestra los resultados de health checks
func (oe *ObservabilityExample) DemoHealthChecks() {
	ctx := context.Background()
	
	oe.logger.Info("Running health checks demo")

	// Ejecutar todas las verificaciones
	results := oe.healthChecker.CheckAll(ctx)
	overallStatus := oe.healthChecker.GetOverallStatus(results)

	oe.logger.Info("Health checks completed",
		zap.String("overall_status", string(overallStatus)),
		zap.Int("checks_count", len(results)),
	)

	// Mostrar resultados detallados
	for name, result := range results {
		oe.logger.Info("Health check result",
			zap.String("check_name", name),
			zap.String("status", string(result.Status)),
			zap.String("message", result.Message),
			zap.Duration("duration", result.Duration),
			zap.Any("metadata", result.Metadata),
		)
	}
}

// DemoMetricsCollection demuestra la recolección de métricas
func (oe *ObservabilityExample) DemoMetricsCollection() {
	oe.logger.Info("Starting metrics collection demo")

	// Simular diferentes eventos para generar métricas
	scenarios := []struct {
		name   string
		action func()
	}{
		{
			"successful_payments",
			func() {
				for i := 0; i < 10; i++ {
					oe.metrics.IncrementPaymentCounter("success", "USD", "credit_card")
					oe.metrics.RecordPaymentLatency(150*time.Millisecond, "success", "USD")
				}
			},
		},
		{
			"failed_payments",
			func() {
				for i := 0; i < 3; i++ {
					oe.metrics.IncrementPaymentCounter("failed", "USD", "credit_card")
					oe.metrics.RecordPaymentLatency(50*time.Millisecond, "failed", "USD")
				}
			},
		},
		{
			"wallet_operations",
			func() {
				for i := 0; i < 15; i++ {
					oe.metrics.IncrementWalletOperationCounter("debit", "USD")
				}
			},
		},
		{
			"retry_attempts",
			func() {
				for i := 0; i < 5; i++ {
					oe.metrics.IncrementRetryCounter("payment", "processing")
				}
			},
		},
	}

	for _, scenario := range scenarios {
		oe.logger.Info("Generating metrics",
			zap.String("scenario", scenario.name),
		)
		scenario.action()
		time.Sleep(100 * time.Millisecond)
	}

	oe.logger.Info("Metrics collection demo completed")
}

// RunCompleteObservabilityDemo ejecuta una demostración completa
func RunCompleteObservabilityDemo() {
	// Configurar dependencias mock
	db := &sql.DB{} // En un caso real, sería una conexión real
	eventStore := &MockEventStore{}
	redisClient := &MockRedisClient{}

	// Crear ejemplo de observabilidad
	example, err := NewObservabilityExample(db, eventStore, redisClient)
	if err != nil {
		log.Fatalf("Failed to create observability example: %v", err)
	}

	fmt.Println("=== Payment System Observability Demo ===")
	fmt.Println()

	// Demo 1: Health Checks
	fmt.Println("1. Health Checks Demo:")
	example.DemoHealthChecks()
	fmt.Println()

	// Demo 2: Metrics Collection
	fmt.Println("2. Metrics Collection Demo:")
	example.DemoMetricsCollection()
	fmt.Println()

	// Demo 3: Payment Processing with Full Observability
	fmt.Println("3. Payment Processing with Observability:")
	example.DemoPaymentProcessingWithObservability()
	fmt.Println()

	fmt.Println("=== Demo Completed ===")
	fmt.Println("Observability endpoints available at:")
	fmt.Println("- Health: http://localhost:8080/health")
	fmt.Println("- Readiness: http://localhost:8080/ready")
	fmt.Println("- Liveness: http://localhost:8080/live")
	fmt.Println("- Metrics: http://localhost:8080/metrics")
}

// MockEventStore implementación mock para demos
type MockEventStore struct{}

// Implementar EventStoreInterface
func (m *MockEventStore) AppendEvent(ctx context.Context, event *Event) error {
	return nil
}

func (m *MockEventStore) GetEvents(ctx context.Context, filter EventFilter) ([]*Event, error) {
	// Retornar algunos eventos mock
	return []*Event{
		{
			ID:            "evt_001",
			AggregateID:   "agg_001",
			EventType:     "PaymentRequested",
			AggregateType: "Payment",
			Data:          map[string]interface{}{"amount": 100.0},
			CreatedAt:     time.Now().Add(-1 * time.Hour),
		},
		{
			ID:            "evt_002",
			AggregateID:   "agg_002",
			EventType:     "PaymentConfirmed",
			AggregateType: "Payment",
			Data:          map[string]interface{}{"amount": 50.0},
			CreatedAt:     time.Now().Add(-30 * time.Minute),
		},
	}, nil
}

func (m *MockEventStore) AppendEvents(ctx context.Context, aggregateID string, events []*Event) error {
	return nil
}

func (m *MockEventStore) SaveSnapshot(ctx context.Context, snapshot *Snapshot) error {
	return nil
}

func (m *MockEventStore) GetSnapshot(ctx context.Context, aggregateID string) (*Snapshot, error) {
	return nil, nil
}

func (m *MockEventStore) Subscribe(ctx context.Context, eventTypes []string) (<-chan *Event, error) {
	ch := make(chan *Event)
	close(ch)
	return ch, nil
}

func (m *MockEventStore) Close() error {
	return nil
}

// MockRedisClient implementación mock para demos
type MockRedisClient struct{}

func (m *MockRedisClient) Ping(ctx context.Context) error {
	return nil
}

func (m *MockRedisClient) Info(ctx context.Context) (string, error) {
	return "redis_version:6.2.0\nused_memory:1024000", nil
}

// Interfaces necesarias para compatibilidad - ahora usando los tipos de metrics.go
