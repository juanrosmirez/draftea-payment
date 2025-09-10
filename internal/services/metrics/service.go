package metrics

import (
	"context"
	"fmt"
	"sync"
	"time"

	"payment-system/internal/events"
	
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
)

// MetricType tipos de métricas
type MetricType string

const (
	MetricTypeCounter   MetricType = "counter"
	MetricTypeGauge     MetricType = "gauge"
	MetricTypeHistogram MetricType = "histogram"
	MetricTypeSummary   MetricType = "summary"
)

// PaymentMetrics métricas específicas de pagos
type PaymentMetrics struct {
	PaymentsTotal        prometheus.Counter
	PaymentsSuccessful   prometheus.Counter
	PaymentsFailed       prometheus.Counter
	PaymentDuration      prometheus.Histogram
	PaymentAmount        prometheus.Histogram
	ActivePayments       prometheus.Gauge
	WalletBalance        prometheus.GaugeVec
	GatewayResponseTime  prometheus.Histogram
	CircuitBreakerState  prometheus.GaugeVec
}

// AlertRule regla de alerta
type AlertRule struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	MetricName  string    `json:"metric_name"`
	Condition   string    `json:"condition"`
	Threshold   float64   `json:"threshold"`
	Duration    time.Duration `json:"duration"`
	Enabled     bool      `json:"enabled"`
	LastFired   time.Time `json:"last_fired"`
	Description string    `json:"description"`
}

// Alert alerta generada
type Alert struct {
	ID          uuid.UUID              `json:"id"`
	RuleID      uuid.UUID              `json:"rule_id"`
	RuleName    string                 `json:"rule_name"`
	MetricName  string                 `json:"metric_name"`
	Value       float64                `json:"value"`
	Threshold   float64                `json:"threshold"`
	Condition   string                 `json:"condition"`
	Severity    string                 `json:"severity"`
	Message     string                 `json:"message"`
	Labels      map[string]string      `json:"labels"`
	Annotations map[string]interface{} `json:"annotations"`
	FiredAt     time.Time              `json:"fired_at"`
}

// Repository interfaz para persistencia de métricas
type Repository interface {
	SaveMetric(ctx context.Context, metricName string, value float64, labels map[string]string, timestamp time.Time) error
	GetMetrics(ctx context.Context, metricName string, from, to time.Time) ([]MetricPoint, error)
	SaveAlert(ctx context.Context, alert *Alert) error
	GetActiveAlerts(ctx context.Context) ([]*Alert, error)
}

// MetricPoint punto de métrica
type MetricPoint struct {
	Timestamp time.Time         `json:"timestamp"`
	Value     float64           `json:"value"`
	Labels    map[string]string `json:"labels"`
}

// Service servicio de métricas
type Service struct {
	repo           Repository
	eventPublisher events.Publisher
	metrics        *PaymentMetrics
	alertRules     map[uuid.UUID]*AlertRule
	alertMutex     sync.RWMutex
	logger         *zap.Logger
}

// NewService crea una nueva instancia del servicio de métricas
func NewService(repo Repository, eventPublisher events.Publisher, logger *zap.Logger) *Service {
	metrics := &PaymentMetrics{
		PaymentsTotal: promauto.NewCounter(prometheus.CounterOpts{
			Name: "payments_total",
			Help: "Total number of payment requests",
		}),
		PaymentsSuccessful: promauto.NewCounter(prometheus.CounterOpts{
			Name: "payments_successful_total",
			Help: "Total number of successful payments",
		}),
		PaymentsFailed: promauto.NewCounter(prometheus.CounterOpts{
			Name: "payments_failed_total",
			Help: "Total number of failed payments",
		}),
		PaymentDuration: promauto.NewHistogram(prometheus.HistogramOpts{
			Name:    "payment_duration_seconds",
			Help:    "Payment processing duration in seconds",
			Buckets: prometheus.DefBuckets,
		}),
		PaymentAmount: promauto.NewHistogram(prometheus.HistogramOpts{
			Name:    "payment_amount",
			Help:    "Payment amounts in cents",
			Buckets: []float64{100, 500, 1000, 5000, 10000, 50000, 100000, 500000, 1000000},
		}),
		ActivePayments: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "active_payments",
			Help: "Number of currently active payments",
		}),
		WalletBalance: *promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "wallet_balance",
			Help: "Current wallet balance by user and currency",
		}, []string{"user_id", "currency"}),
		GatewayResponseTime: promauto.NewHistogram(prometheus.HistogramOpts{
			Name:    "gateway_response_time_seconds",
			Help:    "Gateway response time in seconds",
			Buckets: []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		}),
		CircuitBreakerState: *promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "circuit_breaker_state",
			Help: "Circuit breaker state (0=closed, 1=open, 2=half-open)",
		}, []string{"service", "provider"}),
	}

	return &Service{
		repo:           repo,
		eventPublisher: eventPublisher,
		metrics:        metrics,
		alertRules:     make(map[uuid.UUID]*AlertRule),
		logger:         logger,
	}
}

// RecordPaymentInitiated registra inicio de pago
func (s *Service) RecordPaymentInitiated(ctx context.Context, paymentID uuid.UUID, userID uuid.UUID, amount int64) {
	s.metrics.PaymentsTotal.Inc()
	s.metrics.ActivePayments.Inc()
	s.metrics.PaymentAmount.Observe(float64(amount))

	s.publishMetricEvent(ctx, "payment_initiated", 1, map[string]string{
		"payment_id": paymentID.String(),
		"user_id":    userID.String(),
	})

	s.logger.Debug("payment initiated metric recorded",
		zap.String("payment_id", paymentID.String()),
		zap.Int64("amount", amount))
}

// RecordPaymentCompleted registra pago completado
func (s *Service) RecordPaymentCompleted(ctx context.Context, paymentID uuid.UUID, duration time.Duration) {
	s.metrics.PaymentsSuccessful.Inc()
	s.metrics.ActivePayments.Dec()
	s.metrics.PaymentDuration.Observe(duration.Seconds())

	s.publishMetricEvent(ctx, "payment_completed", 1, map[string]string{
		"payment_id": paymentID.String(),
		"duration":   duration.String(),
	})

	s.logger.Debug("payment completed metric recorded",
		zap.String("payment_id", paymentID.String()),
		zap.Duration("duration", duration))
}

// RecordPaymentFailed registra pago fallido
func (s *Service) RecordPaymentFailed(ctx context.Context, paymentID uuid.UUID, errorCode string) {
	s.metrics.PaymentsFailed.Inc()
	s.metrics.ActivePayments.Dec()

	s.publishMetricEvent(ctx, "payment_failed", 1, map[string]string{
		"payment_id":  paymentID.String(),
		"error_code":  errorCode,
	})

	s.logger.Debug("payment failed metric recorded",
		zap.String("payment_id", paymentID.String()),
		zap.String("error_code", errorCode))
}

// RecordWalletBalance registra saldo de billetera
func (s *Service) RecordWalletBalance(ctx context.Context, userID uuid.UUID, currency string, balance int64) {
	s.metrics.WalletBalance.WithLabelValues(userID.String(), currency).Set(float64(balance))

	s.publishMetricEvent(ctx, "wallet_balance", float64(balance), map[string]string{
		"user_id":  userID.String(),
		"currency": currency,
	})
}

// RecordGatewayResponseTime registra tiempo de respuesta de pasarela
func (s *Service) RecordGatewayResponseTime(ctx context.Context, provider string, duration time.Duration) {
	s.metrics.GatewayResponseTime.Observe(duration.Seconds())

	s.publishMetricEvent(ctx, "gateway_response_time", duration.Seconds(), map[string]string{
		"provider": provider,
	})
}

// RecordCircuitBreakerState registra estado del circuit breaker
func (s *Service) RecordCircuitBreakerState(ctx context.Context, service, provider string, state int) {
	s.metrics.CircuitBreakerState.WithLabelValues(service, provider).Set(float64(state))

	s.publishMetricEvent(ctx, "circuit_breaker_state", float64(state), map[string]string{
		"service":  service,
		"provider": provider,
	})
}

// AddAlertRule agrega una nueva regla de alerta
func (s *Service) AddAlertRule(ctx context.Context, rule *AlertRule) error {
	s.alertMutex.Lock()
	defer s.alertMutex.Unlock()

	rule.ID = uuid.New()
	s.alertRules[rule.ID] = rule

	s.logger.Info("alert rule added",
		zap.String("rule_id", rule.ID.String()),
		zap.String("rule_name", rule.Name),
		zap.String("metric_name", rule.MetricName))

	return nil
}

// CheckAlerts verifica reglas de alerta
func (s *Service) CheckAlerts(ctx context.Context) error {
	s.alertMutex.RLock()
	rules := make([]*AlertRule, 0, len(s.alertRules))
	for _, rule := range s.alertRules {
		if rule.Enabled {
			rules = append(rules, rule)
		}
	}
	s.alertMutex.RUnlock()

	for _, rule := range rules {
		if err := s.evaluateAlertRule(ctx, rule); err != nil {
			s.logger.Error("failed to evaluate alert rule",
				zap.String("rule_id", rule.ID.String()),
				zap.Error(err))
		}
	}

	return nil
}

// evaluateAlertRule evalúa una regla de alerta específica
func (s *Service) evaluateAlertRule(ctx context.Context, rule *AlertRule) error {
	// Obtener métricas recientes
	to := time.Now()
	from := to.Add(-rule.Duration)
	
	metrics, err := s.repo.GetMetrics(ctx, rule.MetricName, from, to)
	if err != nil {
		return fmt.Errorf("failed to get metrics: %w", err)
	}

	if len(metrics) == 0 {
		return nil
	}

	// Evaluar condición
	latestValue := metrics[len(metrics)-1].Value
	shouldAlert := false

	switch rule.Condition {
	case "gt":
		shouldAlert = latestValue > rule.Threshold
	case "lt":
		shouldAlert = latestValue < rule.Threshold
	case "eq":
		shouldAlert = latestValue == rule.Threshold
	case "gte":
		shouldAlert = latestValue >= rule.Threshold
	case "lte":
		shouldAlert = latestValue <= rule.Threshold
	}

	if shouldAlert && time.Since(rule.LastFired) > rule.Duration {
		alert := &Alert{
			ID:         uuid.New(),
			RuleID:     rule.ID,
			RuleName:   rule.Name,
			MetricName: rule.MetricName,
			Value:      latestValue,
			Threshold:  rule.Threshold,
			Condition:  rule.Condition,
			Severity:   s.determineSeverity(rule, latestValue),
			Message:    fmt.Sprintf("Alert: %s - %s %s %.2f (current: %.2f)", rule.Name, rule.MetricName, rule.Condition, rule.Threshold, latestValue),
			Labels:     metrics[len(metrics)-1].Labels,
			FiredAt:    time.Now(),
		}

		// Guardar alerta
		if err := s.repo.SaveAlert(ctx, alert); err != nil {
			return fmt.Errorf("failed to save alert: %w", err)
		}

		// Publicar evento de alerta
		s.publishAlertEvent(ctx, alert)

		// Actualizar última vez que se disparó
		rule.LastFired = time.Now()

		s.logger.Warn("alert triggered",
			zap.String("alert_id", alert.ID.String()),
			zap.String("rule_name", rule.Name),
			zap.Float64("value", latestValue),
			zap.Float64("threshold", rule.Threshold))
	}

	return nil
}

// determineSeverity determina la severidad de una alerta
func (s *Service) determineSeverity(rule *AlertRule, value float64) string {
	deviation := (value - rule.Threshold) / rule.Threshold
	absDeviation := deviation
	if absDeviation < 0 {
		absDeviation = -absDeviation
	}

	switch {
	case absDeviation > 0.5:
		return "critical"
	case absDeviation > 0.2:
		return "warning"
	default:
		return "info"
	}
}

// publishMetricEvent publica evento de métrica
func (s *Service) publishMetricEvent(ctx context.Context, metricName string, value float64, labels map[string]string) {
	event := events.MetricsCollectedEvent{
		BaseEvent: events.BaseEvent{
			ID:          uuid.New(),
			Type:        events.MetricsCollected,
			AggregateID: uuid.New(),
			Version:     1,
			Timestamp:   time.Now(),
		},
		MetricType: string(MetricTypeGauge),
		MetricName: metricName,
		Value:      value,
		Labels:     labels,
	}

	if err := s.eventPublisher.Publish(ctx, event); err != nil {
		s.logger.Error("failed to publish metric event", zap.Error(err))
	}

	// Guardar en repositorio
	s.repo.SaveMetric(ctx, metricName, value, labels, time.Now())
}

// publishAlertEvent publica evento de alerta
func (s *Service) publishAlertEvent(ctx context.Context, alert *Alert) {
	event := events.BaseEvent{
		ID:          uuid.New(),
		Type:        events.AlertTriggered,
		AggregateID: alert.RuleID,
		Version:     1,
		Timestamp:   time.Now(),
		Metadata: map[string]interface{}{
			"alert_id":    alert.ID.String(),
			"rule_name":   alert.RuleName,
			"metric_name": alert.MetricName,
			"value":       alert.Value,
			"threshold":   alert.Threshold,
			"severity":    alert.Severity,
			"message":     alert.Message,
		},
	}

	if err := s.eventPublisher.Publish(ctx, event); err != nil {
		s.logger.Error("failed to publish alert event", zap.Error(err))
	}
}

// GetMetrics obtiene métricas históricas
func (s *Service) GetMetrics(ctx context.Context, metricName string, from, to time.Time) ([]MetricPoint, error) {
	return s.repo.GetMetrics(ctx, metricName, from, to)
}

// GetActiveAlerts obtiene alertas activas
func (s *Service) GetActiveAlerts(ctx context.Context) ([]*Alert, error) {
	return s.repo.GetActiveAlerts(ctx)
}
