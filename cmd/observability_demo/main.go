package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	"payment-system/pkg/observability"
)

func main() {
	// Configurar logger principal
	logger, err := observability.NewZapLogger("info", false)
	if err != nil {
		log.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Sync()

	logger.Info("Starting Payment System Observability Demo")

	// Configurar base de datos (mock para demo)
	db := setupMockDatabase()
	defer db.Close()

	// Configurar Event Store (mock para demo)
	eventStore := &observability.MockEventStore{}

	// Configurar Redis (mock para demo)
	redisClient := &observability.MockRedisClient{}

	// Configurar métricas
	metrics := observability.NewMetricsCollector("payment_system")

	// Configurar health checks
	healthChecker := observability.SetupHealthChecks(db, eventStore, redisClient, logger)
	healthHandler := observability.NewHealthHandler(healthChecker, logger)

	// Configurar servidor HTTP
	mux := http.NewServeMux()

	// Endpoints de health checks
	mux.HandleFunc("/health", healthHandler.HealthzHandler)
	mux.HandleFunc("/healthz", healthHandler.HealthzHandler)
	mux.HandleFunc("/ready", healthHandler.ReadyHandler)
	mux.HandleFunc("/live", healthHandler.LiveHandler)

	// Endpoint de métricas
	mux.Handle("/metrics", promhttp.Handler())

	// Endpoint de demo
	mux.HandleFunc("/demo", func(w http.ResponseWriter, r *http.Request) {
		runDemoEndpoint(w, r, logger, metrics)
	})

	// Middleware de observabilidad
	handler := withObservabilityMiddleware(mux, logger, metrics)

	// Configurar servidor
	server := &http.Server{
		Addr:    ":8081",
		Handler: handler,
	}

	// Canal para señales del sistema
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Iniciar servidor en goroutine
	go func() {
		logger.Info("Starting HTTP server",
			zap.String("port", "8081"),
			zap.Strings("endpoints", []string{"/health", "/ready", "/live", "/metrics", "/demo"}),
		)

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("Server failed to start", zap.Error(err))
		}
	}()

	// Ejecutar demo en background
	ctx := context.Background()
	go runBackgroundDemo(ctx, logger, metrics)

	// Esperar señal de terminación
	<-sigChan
	logger.Info("Received shutdown signal")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Error("Server shutdown failed", zap.Error(err))
	} else {
		logger.Info("Server shutdown completed")
	}
}

// setupMockDatabase configura una base de datos mock para la demo
func setupMockDatabase() *sql.DB {
	// En un entorno real, esto sería una conexión real a PostgreSQL
	// Para la demo, usamos una conexión mock que no falla
	db, _ := sql.Open("postgres", "mock://connection")
	return db
}

// withObservabilityMiddleware añade middleware de logging y métricas
func withObservabilityMiddleware(
	next http.Handler,
	logger observability.StructuredLogger,
	metrics observability.MetricsCollector,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Crear contexto con trace ID
		ctx := observability.WithTraceID(r.Context(), observability.GenerateTraceID())
		r = r.WithContext(ctx)

		// Wrapper para capturar status code
		wrapper := &responseWrapper{ResponseWriter: w, statusCode: 200}

		// Log request
		logger.WithContext(ctx).Info("HTTP request started",
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
		metrics.IncrementHTTPRequestCounter(r.Method, r.URL.Path, wrapper.statusCode)
		metrics.RecordHTTPRequestDuration(r.Method, r.URL.Path, wrapper.statusCode, duration)

		// Log response
		logger.WithContext(ctx).Info("HTTP request completed",
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

// runDemoEndpoint maneja el endpoint de demo
func runDemoEndpoint(
	w http.ResponseWriter,
	r *http.Request,
	logger observability.StructuredLogger,
	metrics observability.MetricsCollector,
) {
	ctx := r.Context()

	logger.WithContext(ctx).Info("Demo endpoint called")

	// Simular procesamiento de pago
	processPaymentDemo(ctx, logger, metrics)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{
		"message": "Demo payment processed successfully",
		"timestamp": "` + time.Now().UTC().Format(time.RFC3339) + `",
		"trace_id": "` + observability.GetTraceID(ctx) + `"
	}`))
}

// processPaymentDemo simula el procesamiento de un pago con observabilidad
func processPaymentDemo(
	ctx context.Context,
	logger observability.StructuredLogger,
	metrics observability.MetricsCollector,
) {
	start := time.Now()

	// Añadir contexto de pago
	paymentID := "pay_demo_" + observability.GenerateTraceID()[:8]
	userID := "user_demo_123"
	ctx = observability.WithPaymentID(ctx, paymentID)
	ctx = observability.WithUserID(ctx, userID)

	logger.WithContext(ctx).Info("Processing demo payment",
		zap.String("payment_id", paymentID),
		zap.String("user_id", userID),
		zap.Float64("amount", 99.99),
	)

	// Incrementar métricas
	metrics.IncrementPaymentCounter("attempted", "USD", "credit_card")

	// Simular procesamiento
	time.Sleep(100 * time.Millisecond)

	// Simular éxito (90% de probabilidad)
	if time.Now().UnixNano()%10 < 9 {
		// Pago exitoso
		duration := time.Since(start)

		logger.WithContext(ctx).Info("Demo payment processed successfully",
			zap.Duration("processing_time", duration),
		)

		metrics.IncrementPaymentCounter("success", "USD", "credit_card")
		metrics.RecordPaymentLatency(duration, "success", "USD")

		// Simular actualización de billetera
		updateWalletDemo(ctx, logger, metrics, userID, 99.99)
	} else {
		// Pago fallido
		duration := time.Since(start)

		logger.WithContext(ctx).Error("Demo payment failed",
			zap.String("error", "insufficient_funds"),
			zap.Duration("processing_time", duration),
		)

		metrics.IncrementPaymentCounter("failed", "USD", "credit_card")
		metrics.RecordPaymentLatency(duration, "failed", "USD")
	}
}

// updateWalletDemo simula actualización de billetera
func updateWalletDemo(
	ctx context.Context,
	logger observability.StructuredLogger,
	metrics observability.MetricsCollector,
	userID string,
	amount float64,
) {
	start := time.Now()

	logger.WithContext(ctx).Info("Updating wallet",
		zap.String("user_id", userID),
		zap.Float64("amount", amount),
		zap.String("operation", "debit"),
	)

	// Simular operación
	time.Sleep(50 * time.Millisecond)

	duration := time.Since(start)

	// Actualizar métricas
	metrics.IncrementWalletOperationCounter("debit", "USD")

	// Simular balance
	newBalance := 500.0 - amount
	metrics.RecordWalletBalance(userID, "USD", newBalance)

	logger.WithContext(ctx).Info("Wallet updated successfully",
		zap.Float64("new_balance", newBalance),
		zap.Duration("operation_time", duration),
	)
}

// runBackgroundDemo ejecuta demos periódicos en background
func runBackgroundDemo(
	ctx context.Context,
	logger observability.StructuredLogger,
	metrics observability.MetricsCollector,
) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		// Simular métricas de sistema
		generateSystemMetrics(ctx, logger, metrics)
	}
}

// generateSystemMetrics genera métricas de sistema periódicas
func generateSystemMetrics(
	_ context.Context,
	logger observability.StructuredLogger,
	metrics observability.MetricsCollector,
) {
	// Simular eventos del sistema
	events := []struct {
		eventType string
		action    func()
	}{
		{
			"payment_success",
			func() {
				metrics.IncrementPaymentCounter("success", "USD", "credit_card")
				metrics.RecordPaymentLatency(125*time.Millisecond, "success", "USD")
			},
		},
		{
			"wallet_operation",
			func() {
				metrics.IncrementWalletOperationCounter("debit", "USD")
			},
		},
		{
			"event_store_operation",
			func() {
				metrics.IncrementEventCounter("PaymentProcessed", "Payment")
				metrics.RecordEventProcessingLatency(15*time.Millisecond, "PaymentProcessed")
			},
		},
	}

	// Log the metrics generation
	logger.Info("Generating system metrics", 
		zap.Int("event_count", len(events)),
		zap.Time("timestamp", time.Now()),
	)

	// Generar eventos aleatorios
	for _, event := range events {
		if time.Now().UnixNano()%3 == 0 { // 33% probabilidad
			event.action()
		}
	}
}
