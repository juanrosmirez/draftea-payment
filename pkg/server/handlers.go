package server

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"github.com/google/uuid"
	
	"payment-system/internal/events"
	"payment-system/internal/health"
	"payment-system/internal/saga"
	"payment-system/internal/services/payment"
	"payment-system/internal/services/wallet"
	"payment-system/internal/services/metrics"
	"payment-system/pkg/database"
)

// Dependencies holds all service dependencies
type Dependencies struct {
	PaymentService *payment.Service
	WalletService  *wallet.Service
	MetricsService *metrics.Service
	HealthChecker  *health.HealthChecker
}

// mockGatewayService is a mock implementation for testing
type mockGatewayService struct{}

func (m *mockGatewayService) ProcessPayment(ctx context.Context, payment *payment.Payment) (string, error) {
	return "mock-txn-" + payment.ID.String(), nil
}

// HealthHandler maneja el endpoint de health check
func HealthHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "healthy",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"service": "payment-system",
		"version": "1.0.0",
	})
}

// SetupRoutes configures all HTTP routes
func SetupRoutes(router *gin.Engine, deps *Dependencies) {
	// Health check endpoints
	router.GET("/health", deps.HealthChecker.CheckHealth)
	router.GET("/ready", deps.HealthChecker.CheckReadiness)
	router.GET("/live", deps.HealthChecker.CheckLiveness)

	// API routes
	api := router.Group("/api/v1")
	{
		// Payment routes
		payments := api.Group("/payments")
		{
			payments.POST("", CreatePaymentHandler(deps.PaymentService))
			payments.GET("/:id", GetPaymentHandler(deps.PaymentService))
			// payments.GET("", listPaymentsHandler(deps.PaymentService))
			// payments.POST("/:id/cancel", cancelPaymentHandler(deps.PaymentService))
		}

		// Wallet routes
		wallets := api.Group("/wallets")
		{
			wallets.GET("/:user_id/:currency", GetWalletHandler(deps.WalletService))
			// wallets.POST("/:id/deduct", deductFromWalletHandler(deps.WalletService))
			// wallets.POST("/:id/refund", refundToWalletHandler(deps.WalletService))
		}

		// Metrics routes
		metrics := api.Group("/metrics")
		{
			metrics.GET("", GetMetricsHandler(deps.MetricsService))
			metrics.GET("/alerts", GetAlertsHandler(deps.MetricsService))
		}
	}
}

// NewDependencies creates all service dependencies
func NewDependencies(dbConn *sql.DB, redis *redis.Client, eventPublisher events.Publisher, logger *zap.Logger) *Dependencies {
	// Create database wrapper
	db := &database.DB{DB: dbConn}
	
	// Create repositories
	paymentRepo := database.NewPaymentRepository(db)
	walletRepo := database.NewWalletRepository(db)
	metricsRepo := database.NewMetricsRepository(db)
	
	// Create gateway service (mock for now)
	gatewayService := &mockGatewayService{}
	
	// Create wallet service
	walletService := wallet.NewService(walletRepo, eventPublisher, logger)
	
	// Create payment service
	paymentService := payment.NewService(paymentRepo, walletService, gatewayService, eventPublisher, logger)
	
	// Create metrics service
	metricsService := metrics.NewService(metricsRepo, eventPublisher, logger)
	
	return &Dependencies{
		PaymentService: paymentService,
		WalletService:  walletService,
		MetricsService: metricsService,
		HealthChecker:  health.NewHealthChecker(dbConn, redis),
	}
}

// ReadinessHandler maneja el endpoint de readiness
func ReadinessHandler(c *gin.Context) {
	// Aquí podrías agregar verificaciones de dependencias
	c.JSON(http.StatusOK, gin.H{
		"status": "ready",
		"checks": gin.H{
			"database": "ok",
			"redis":    "ok",
			"kafka":    "ok",
		},
	})
}

// LivenessHandler maneja el endpoint de liveness
func LivenessHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "alive",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"service": "payment-system",
	})
}

// MetricsHandler maneja el endpoint de métricas de Prometheus
func MetricsHandler(c *gin.Context) {
	c.Header("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	c.String(http.StatusOK, `# HELP payment_system_info Information about the payment system
# TYPE payment_system_info gauge
payment_system_info{version="1.0.0",service="payment-system"} 1

# HELP payment_requests_total Total number of payment requests
# TYPE payment_requests_total counter
payment_requests_total 0

# HELP payment_requests_success_total Total number of successful payments
# TYPE payment_requests_success_total counter
payment_requests_success_total 0

# HELP payment_requests_failed_total Total number of failed payments
# TYPE payment_requests_failed_total counter
payment_requests_failed_total 0

# HELP wallet_balance_usd Current wallet balance in USD
# TYPE wallet_balance_usd gauge
wallet_balance_usd 100000

# HELP wallet_balance_eur Current wallet balance in EUR
# TYPE wallet_balance_eur gauge
wallet_balance_eur 200000
`)
}

// CreatePaymentHandler maneja la creación de pagos
func CreatePaymentHandler(paymentService *payment.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req payment.PaymentRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Validaciones básicas
		if req.Amount <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "amount must be positive"})
			return
		}

		if req.Currency == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "currency is required"})
			return
		}

		// Validar monedas soportadas
		supportedCurrencies := map[string]bool{
			"USD": true,
			"EUR": true,
			"GBP": true,
			"JPY": true,
			"CAD": true,
			"AUD": true,
		}
		if !supportedCurrencies[req.Currency] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "currency not supported"})
			return
		}

		if req.UserID == uuid.Nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "user_id is required"})
			return
		}

		payment, err := paymentService.ProcessPayment(c.Request.Context(), &req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusCreated, payment)
	}
}

// GetPaymentHandler maneja la obtención de un pago
func GetPaymentHandler(paymentService *payment.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		id, err := uuid.Parse(idStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payment ID"})
			return
		}

		payment, err := paymentService.GetPayment(c.Request.Context(), id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "payment not found"})
			return
		}

		c.JSON(http.StatusOK, payment)
	}
}

// GetWalletHandler maneja la obtención de una billetera
func GetWalletHandler(walletService *wallet.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		userIDStr := c.Param("user_id")
		userID, err := uuid.Parse(userIDStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
			return
		}

		currency := c.Param("currency")
		if currency == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "currency is required"})
			return
		}

		wallet, err := walletService.GetWallet(c.Request.Context(), userID, currency)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "wallet not found"})
			return
		}

		c.JSON(http.StatusOK, wallet)
	}
}

// GetTransactionHistoryHandler maneja la obtención del historial de transacciones
func GetTransactionHistoryHandler(walletService *wallet.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		userIDStr := c.Param("user_id")
		userID, err := uuid.Parse(userIDStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
			return
		}

		currency := c.Param("currency")
		if currency == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "currency is required"})
			return
		}

		transactions, err := walletService.GetTransactionHistory(c.Request.Context(), userID, currency)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"transactions": transactions,
			"count":        len(transactions),
		})
	}
}

// GetMetricsHandler maneja la obtención de métricas
func GetMetricsHandler(metricsService *metrics.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		metricName := c.Query("metric")
		if metricName == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "metric parameter is required"})
			return
		}

		// Parsear parámetros de tiempo
		fromStr := c.DefaultQuery("from", time.Now().Add(-24*time.Hour).Format(time.RFC3339))
		toStr := c.DefaultQuery("to", time.Now().Format(time.RFC3339))

		from, err := time.Parse(time.RFC3339, fromStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid from timestamp"})
			return
		}

		to, err := time.Parse(time.RFC3339, toStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid to timestamp"})
			return
		}

		metrics, err := metricsService.GetMetrics(c.Request.Context(), metricName, from, to)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"metric_name": metricName,
			"from":        from,
			"to":          to,
			"points":      metrics,
			"count":       len(metrics),
		})
	}
}

// GetAlertsHandler maneja la obtención de alertas
func GetAlertsHandler(metricsService *metrics.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		alerts, err := metricsService.GetActiveAlerts(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"alerts": alerts,
			"count":  len(alerts),
		})
	}
}

// CreateAlertRuleRequest estructura para crear reglas de alerta
type CreateAlertRuleRequest struct {
	Name        string  `json:"name" binding:"required"`
	MetricName  string  `json:"metric_name" binding:"required"`
	Condition   string  `json:"condition" binding:"required"`
	Threshold   float64 `json:"threshold" binding:"required"`
	Duration    string  `json:"duration" binding:"required"`
	Description string  `json:"description"`
}

// CreateAlertRuleHandler maneja la creación de reglas de alerta
func CreateAlertRuleHandler(metricsService *metrics.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req CreateAlertRuleRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Validar condición
		validConditions := map[string]bool{
			"gt": true, "lt": true, "eq": true, "gte": true, "lte": true,
		}
		if !validConditions[req.Condition] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid condition"})
			return
		}

		// Parsear duración
		duration, err := time.ParseDuration(req.Duration)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid duration format"})
			return
		}

		rule := &metrics.AlertRule{
			Name:        req.Name,
			MetricName:  req.MetricName,
			Condition:   req.Condition,
			Threshold:   req.Threshold,
			Duration:    duration,
			Enabled:     true,
			Description: req.Description,
		}

		if err := metricsService.AddAlertRule(c.Request.Context(), rule); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusCreated, rule)
	}
}

// GetSagaHandler maneja la obtención de una saga
func GetSagaHandler(sagaOrchestrator *saga.Orchestrator) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		id, err := uuid.Parse(idStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid saga ID"})
			return
		}

		saga, err := sagaOrchestrator.GetSaga(c.Request.Context(), id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "saga not found"})
			return
		}

		c.JSON(http.StatusOK, saga)
	}
}
