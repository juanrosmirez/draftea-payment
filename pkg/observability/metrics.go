package observability

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

// MetricsCollector define la interfaz para recolección de métricas
type MetricsCollector interface {
	// Payment metrics
	IncrementPaymentCounter(status, currency, paymentMethod string)
	RecordPaymentLatency(duration time.Duration, status, currency string)
	IncrementRetryCounter(service, operation string)
	
	// Wallet metrics
	IncrementWalletOperationCounter(operation, currency string)
	RecordWalletBalance(userID, currency string, balance float64)
	IncrementConcurrencyConflictCounter(strategy string)
	
	// Event Store metrics
	IncrementEventCounter(eventType, aggregateType string)
	RecordEventProcessingLatency(duration time.Duration, eventType string)
	IncrementDLQCounter(reason string)
	
	// System metrics
	RecordActiveConnections(service string, count int)
	IncrementErrorCounter(service, errorType string)
	
	// HTTP metrics
	RecordHTTPRequestDuration(method, path string, statusCode int, duration time.Duration)
	IncrementHTTPRequestCounter(method, path string, statusCode int)
}

// PrometheusMetrics implementa MetricsCollector usando Prometheus
type PrometheusMetrics struct {
	// Payment metrics
	paymentCounter    *prometheus.CounterVec
	paymentLatency    *prometheus.HistogramVec
	retryCounter      *prometheus.CounterVec
	
	// Wallet metrics
	walletOperationCounter    *prometheus.CounterVec
	walletBalanceGauge       *prometheus.GaugeVec
	concurrencyConflictCounter *prometheus.CounterVec
	
	// Event Store metrics
	eventCounter              *prometheus.CounterVec
	eventProcessingLatency    *prometheus.HistogramVec
	dlqCounter               *prometheus.CounterVec
	
	// System metrics
	activeConnectionsGauge   *prometheus.GaugeVec
	errorCounter            *prometheus.CounterVec
	
	// HTTP metrics
	httpRequestDuration     *prometheus.HistogramVec
	httpRequestCounter      *prometheus.CounterVec
	
	registry *prometheus.Registry
}

// NewMetricsCollector crea un nuevo collector de métricas (alias para NewPrometheusMetrics)
func NewMetricsCollector(namespace string) MetricsCollector {
	return NewPrometheusMetrics(namespace)
}

// NewPrometheusMetrics crea un nuevo collector de métricas Prometheus
func NewPrometheusMetrics(namespace string) *PrometheusMetrics {
	registry := prometheus.NewRegistry()
	
	pm := &PrometheusMetrics{
		registry: registry,
		
		// Payment metrics
		paymentCounter: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "payment",
				Name:      "total",
				Help:      "Total number of payment attempts",
			},
			[]string{"status", "currency", "payment_method"},
		),
		
		paymentLatency: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: "payment",
				Name:      "duration_seconds",
				Help:      "Payment processing duration in seconds",
				Buckets:   []float64{0.1, 0.5, 1.0, 2.0, 5.0, 10.0, 30.0, 60.0},
			},
			[]string{"status", "currency"},
		),
		
		retryCounter: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "retry",
				Name:      "total",
				Help:      "Total number of retry attempts",
			},
			[]string{"service", "operation"},
		),
		
		// Wallet metrics
		walletOperationCounter: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "wallet",
				Name:      "operations_total",
				Help:      "Total number of wallet operations",
			},
			[]string{"operation", "currency"},
		),
		
		walletBalanceGauge: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: "wallet",
				Name:      "balance",
				Help:      "Current wallet balance by user and currency",
			},
			[]string{"user_id", "currency"},
		),
		
		concurrencyConflictCounter: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "concurrency",
				Name:      "conflicts_total",
				Help:      "Total number of concurrency conflicts",
			},
			[]string{"strategy"},
		),
		
		// Event Store metrics
		eventCounter: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "eventstore",
				Name:      "events_total",
				Help:      "Total number of events processed",
			},
			[]string{"event_type", "aggregate_type"},
		),
		
		eventProcessingLatency: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: "eventstore",
				Name:      "processing_duration_seconds",
				Help:      "Event processing duration in seconds",
				Buckets:   []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1.0, 5.0},
			},
			[]string{"event_type"},
		),
		
		dlqCounter: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "eventstore",
				Name:      "dlq_events_total",
				Help:      "Total number of events sent to Dead Letter Queue",
			},
			[]string{"reason"},
		),
		
		// System metrics
		activeConnectionsGauge: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: "system",
				Name:      "active_connections",
				Help:      "Number of active connections by service",
			},
			[]string{"service"},
		),
		
		errorCounter: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "system",
				Name:      "errors_total",
				Help:      "Total number of errors by service and type",
			},
			[]string{"service", "error_type"},
		),
		
		// HTTP metrics
		httpRequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: "http",
				Name:      "request_duration_seconds",
				Help:      "HTTP request duration in seconds",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"method", "path", "status_code"},
		),
		
		httpRequestCounter: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "http",
				Name:      "requests_total",
				Help:      "Total number of HTTP requests",
			},
			[]string{"method", "path", "status_code"},
		),
	}
	
	// Registrar todas las métricas
	registry.MustRegister(
		pm.paymentCounter,
		pm.paymentLatency,
		pm.retryCounter,
		pm.walletOperationCounter,
		pm.walletBalanceGauge,
		pm.concurrencyConflictCounter,
		pm.eventCounter,
		pm.eventProcessingLatency,
		pm.dlqCounter,
		pm.activeConnectionsGauge,
		pm.errorCounter,
		pm.httpRequestDuration,
		pm.httpRequestCounter,
	)
	
	return pm
}

// Payment metrics implementation
func (pm *PrometheusMetrics) IncrementPaymentCounter(status, currency, paymentMethod string) {
	pm.paymentCounter.WithLabelValues(status, currency, paymentMethod).Inc()
}

func (pm *PrometheusMetrics) RecordPaymentLatency(duration time.Duration, status, currency string) {
	pm.paymentLatency.WithLabelValues(status, currency).Observe(duration.Seconds())
}

func (pm *PrometheusMetrics) IncrementRetryCounter(service, operation string) {
	pm.retryCounter.WithLabelValues(service, operation).Inc()
}

// Wallet metrics implementation
func (pm *PrometheusMetrics) IncrementWalletOperationCounter(operation, currency string) {
	pm.walletOperationCounter.WithLabelValues(operation, currency).Inc()
}

func (pm *PrometheusMetrics) RecordWalletBalance(userID, currency string, balance float64) {
	pm.walletBalanceGauge.WithLabelValues(userID, currency).Set(balance)
}

func (pm *PrometheusMetrics) IncrementConcurrencyConflictCounter(strategy string) {
	pm.concurrencyConflictCounter.WithLabelValues(strategy).Inc()
}

// Event Store metrics implementation
func (pm *PrometheusMetrics) IncrementEventCounter(eventType, aggregateType string) {
	pm.eventCounter.WithLabelValues(eventType, aggregateType).Inc()
}

func (pm *PrometheusMetrics) RecordEventProcessingLatency(duration time.Duration, eventType string) {
	pm.eventProcessingLatency.WithLabelValues(eventType).Observe(duration.Seconds())
}

func (pm *PrometheusMetrics) IncrementDLQCounter(reason string) {
	pm.dlqCounter.WithLabelValues(reason).Inc()
}

// System metrics implementation
func (pm *PrometheusMetrics) RecordActiveConnections(service string, count int) {
	pm.activeConnectionsGauge.WithLabelValues(service).Set(float64(count))
}

func (pm *PrometheusMetrics) IncrementErrorCounter(service, errorType string) {
	pm.errorCounter.WithLabelValues(service, errorType).Inc()
}

// HTTP metrics implementation
func (pm *PrometheusMetrics) RecordHTTPRequestDuration(method, path string, statusCode int, duration time.Duration) {
	pm.httpRequestDuration.WithLabelValues(method, path, strconv.Itoa(statusCode)).Observe(duration.Seconds())
}

func (pm *PrometheusMetrics) IncrementHTTPRequestCounter(method, path string, statusCode int) {
	pm.httpRequestCounter.WithLabelValues(method, path, strconv.Itoa(statusCode)).Inc()
}

// GetRegistry retorna el registry de Prometheus para exposición
func (pm *PrometheusMetrics) GetRegistry() *prometheus.Registry {
	return pm.registry
}

// MetricsHandler crea un handler HTTP para el endpoint /metrics
func (pm *PrometheusMetrics) MetricsHandler() http.Handler {
	return promhttp.HandlerFor(pm.registry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	})
}

// InstrumentedPaymentService envuelve el servicio de pagos con métricas
type InstrumentedPaymentService struct {
	service PaymentServiceInterface
	metrics MetricsCollector
	logger  StructuredLogger
}

// PaymentServiceInterface define la interfaz del servicio de pagos
type PaymentServiceInterface interface {
	ProcessPayment(ctx context.Context, request PaymentRequest) error
}

// PaymentRequest representa una solicitud de pago
type PaymentRequest struct {
	PaymentID     string  `json:"payment_id"`
	UserID        string  `json:"user_id"`
	Amount        float64 `json:"amount"`
	Currency      string  `json:"currency"`
	PaymentMethod string  `json:"payment_method"`
}

// NewInstrumentedPaymentService crea un servicio de pagos instrumentado
func NewInstrumentedPaymentService(service PaymentServiceInterface, metrics MetricsCollector, logger StructuredLogger) *InstrumentedPaymentService {
	return &InstrumentedPaymentService{
		service: service,
		metrics: metrics,
		logger:  logger,
	}
}

// ProcessPayment procesa un pago con instrumentación de métricas
func (ips *InstrumentedPaymentService) ProcessPayment(ctx context.Context, request PaymentRequest) error {
	start := time.Now()
	
	// Log inicio del pago
	ips.logger.WithContext(ctx).Info("Payment processing started",
		zap.String("payment_id", request.PaymentID),
		zap.String("user_id", request.UserID),
		zap.Float64("amount", request.Amount),
		zap.String("currency", request.Currency),
	)
	
	// Procesar pago
	err := ips.service.ProcessPayment(ctx, request)
	
	// Calcular duración
	duration := time.Since(start)
	
	// Determinar status
	status := "success"
	if err != nil {
		status = "failed"
		ips.metrics.IncrementErrorCounter("payment", "processing_error")
	}
	
	// Registrar métricas
	ips.metrics.IncrementPaymentCounter(status, request.Currency, request.PaymentMethod)
	ips.metrics.RecordPaymentLatency(duration, status, request.Currency)
	
	// Log resultado
	if err != nil {
		ips.logger.WithContext(ctx).Error("Payment processing failed",
			zap.String("payment_id", request.PaymentID),
			zap.Error(err),
			zap.Duration("duration", duration),
		)
	} else {
		ips.logger.WithContext(ctx).Info("Payment processing completed",
			zap.String("payment_id", request.PaymentID),
			zap.Duration("duration", duration),
		)
	}
	
	return err
}

// InstrumentedWalletService envuelve el servicio de billetera con métricas
type InstrumentedWalletService struct {
	service WalletServiceInterface
	metrics MetricsCollector
	logger  StructuredLogger
}

// WalletServiceInterface define la interfaz del servicio de billetera
type WalletServiceInterface interface {
	CheckBalance(ctx context.Context, userID, currency string) (float64, error)
	DeductFunds(ctx context.Context, userID, currency string, amount float64, reason string) error
	RefundFunds(ctx context.Context, userID, currency string, amount float64, reason string) error
}

// NewInstrumentedWalletService crea un servicio de billetera instrumentado
func NewInstrumentedWalletService(service WalletServiceInterface, metrics MetricsCollector, logger StructuredLogger) *InstrumentedWalletService {
	return &InstrumentedWalletService{
		service: service,
		metrics: metrics,
		logger:  logger,
	}
}

// DeductFunds deduce fondos con instrumentación
func (iws *InstrumentedWalletService) DeductFunds(ctx context.Context, userID, currency string, amount float64, reason string) error {
	start := time.Now()
	
	// Obtener balance previo para métricas
	previousBalance, _ := iws.service.CheckBalance(ctx, userID, currency)
	
	// Ejecutar operación
	err := iws.service.DeductFunds(ctx, userID, currency, amount, reason)
	
	duration := time.Since(start)
	
	// Registrar métricas
	if err != nil {
		iws.metrics.IncrementErrorCounter("wallet", "deduction_error")
		if contains(err.Error(), "insufficient") {
			iws.metrics.IncrementErrorCounter("wallet", "insufficient_funds")
		}
		if contains(err.Error(), "version conflict") {
			iws.metrics.IncrementConcurrencyConflictCounter("optimistic")
		}
	} else {
		iws.metrics.IncrementWalletOperationCounter("deduct", currency)
		
		// Actualizar balance en métricas
		newBalance := previousBalance - amount
		iws.metrics.RecordWalletBalance(userID, currency, newBalance)
	}
	
	// Log operación
	if err != nil {
		iws.logger.WithContext(ctx).Error("Wallet deduction failed",
			zap.String("user_id", userID),
			zap.String("currency", currency),
			zap.Float64("amount", amount),
			zap.Error(err),
			zap.Duration("duration", duration),
		)
	} else {
		iws.logger.WithContext(ctx).Info("Wallet deduction successful",
			zap.String("user_id", userID),
			zap.String("currency", currency),
			zap.Float64("amount", amount),
			zap.Float64("previous_balance", previousBalance),
			zap.Duration("duration", duration),
		)
	}
	
	return err
}

// CheckBalance verifica saldo con instrumentación
func (iws *InstrumentedWalletService) CheckBalance(ctx context.Context, userID, currency string) (float64, error) {
	balance, err := iws.service.CheckBalance(ctx, userID, currency)
	
	if err != nil {
		iws.metrics.IncrementErrorCounter("wallet", "balance_check_error")
	} else {
		// Actualizar gauge de balance
		iws.metrics.RecordWalletBalance(userID, currency, balance)
	}
	
	return balance, err
}

// RefundFunds reembolsa fondos con instrumentación
func (iws *InstrumentedWalletService) RefundFunds(ctx context.Context, userID, currency string, amount float64, reason string) error {
	start := time.Now()
	
	previousBalance, _ := iws.service.CheckBalance(ctx, userID, currency)
	
	err := iws.service.RefundFunds(ctx, userID, currency, amount, reason)
	
	duration := time.Since(start)
	
	if err != nil {
		iws.metrics.IncrementErrorCounter("wallet", "refund_error")
	} else {
		iws.metrics.IncrementWalletOperationCounter("refund", currency)
		
		// Actualizar balance
		newBalance := previousBalance + amount
		iws.metrics.RecordWalletBalance(userID, currency, newBalance)
	}
	
	// Log operación
	if err != nil {
		iws.logger.WithContext(ctx).Error("Wallet refund failed",
			zap.String("user_id", userID),
			zap.Error(err),
			zap.Duration("duration", duration),
		)
	} else {
		iws.logger.WithContext(ctx).Info("Wallet refund successful",
			zap.String("user_id", userID),
			zap.Float64("amount", amount),
			zap.Duration("duration", duration),
		)
	}
	
	return err
}

// MetricsMiddleware proporciona middleware para métricas HTTP
func MetricsMiddleware(metrics MetricsCollector) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			
			// Wrapper para capturar status code
			wrapped := &responseWriter{ResponseWriter: w, statusCode: 200}
			
			// Procesar request
			next.ServeHTTP(wrapped, r)
			
			// Registrar métricas
			duration := time.Since(start)
			metrics.RecordHTTPRequestDuration(r.Method, r.URL.Path, wrapped.statusCode, duration)
			metrics.IncrementHTTPRequestCounter(r.Method, r.URL.Path, wrapped.statusCode)
		})
	}
}

// EventStoreMetricsWrapper envuelve el Event Store con métricas
type EventStoreMetricsWrapper struct {
	eventStore EventStoreInterface
	metrics    MetricsCollector
	logger     StructuredLogger
}

// EventStoreInterface define la interfaz del Event Store
type EventStoreInterface interface {
	AppendEvent(ctx context.Context, event *Event) error
	GetEvents(ctx context.Context, filter EventFilter) ([]*Event, error)
	AppendEvents(ctx context.Context, aggregateID string, events []*Event) error
	SaveSnapshot(ctx context.Context, snapshot *Snapshot) error
	GetSnapshot(ctx context.Context, aggregateID string) (*Snapshot, error)
	Subscribe(ctx context.Context, eventTypes []string) (<-chan *Event, error)
	Close() error
}

// Event representa un evento en el Event Store
type Event struct {
	ID            string                 `json:"id"`
	EventType     string                 `json:"event_type"`
	AggregateType string                 `json:"aggregate_type"`
	AggregateID   string                 `json:"aggregate_id"`
	Data          map[string]interface{} `json:"data"`
	CreatedAt     time.Time              `json:"created_at"`
}

// EventFilter representa filtros para consulta de eventos
type EventFilter struct {
	EventTypes []string `json:"event_types"`
}

// Snapshot representa un snapshot de agregado
type Snapshot struct {
	AggregateID string                 `json:"aggregate_id"`
	Data        map[string]interface{} `json:"data"`
	Version     int                    `json:"version"`
	CreatedAt   time.Time              `json:"created_at"`
}

// NewEventStoreMetricsWrapper crea un wrapper con métricas para Event Store
func NewEventStoreMetricsWrapper(eventStore EventStoreInterface, metrics MetricsCollector, logger StructuredLogger) *EventStoreMetricsWrapper {
	return &EventStoreMetricsWrapper{
		eventStore: eventStore,
		metrics:    metrics,
		logger:     logger,
	}
}

// AppendEvent almacena un evento con instrumentación
func (esw *EventStoreMetricsWrapper) AppendEvent(ctx context.Context, event *Event) error {
	start := time.Now()
	
	err := esw.eventStore.AppendEvent(ctx, event)
	
	duration := time.Since(start)
	
	// Registrar métricas
	if err != nil {
		esw.metrics.IncrementErrorCounter("eventstore", "append_error")
	} else {
		esw.metrics.IncrementEventCounter(event.EventType, event.AggregateType)
		esw.metrics.RecordEventProcessingLatency(duration, event.EventType)
	}
	
	return err
}

// GetEvents obtiene eventos con instrumentación
func (esw *EventStoreMetricsWrapper) GetEvents(ctx context.Context, filter EventFilter) ([]*Event, error) {
	start := time.Now()
	
	events, err := esw.eventStore.GetEvents(ctx, filter)
	
	duration := time.Since(start)
	
	if err != nil {
		esw.metrics.IncrementErrorCounter("eventstore", "query_error")
	} else {
		// Registrar métricas por tipo de evento obtenido
		for _, event := range events {
			esw.metrics.IncrementEventCounter(event.EventType, event.AggregateType)
		}
		esw.metrics.RecordEventProcessingLatency(duration, "query")
	}
	
	return events, err
}

// contains verifica si una cadena contiene una subcadena
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
