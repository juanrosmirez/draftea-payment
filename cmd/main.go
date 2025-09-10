package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	"payment-system/internal/config"
	"payment-system/internal/events"
	"payment-system/internal/saga"
	"payment-system/internal/services/gateway"
	"payment-system/internal/services/metrics"
	"payment-system/internal/services/payment"
	"payment-system/internal/services/wallet"
	"payment-system/pkg/database"
	"payment-system/pkg/logger"
	"payment-system/pkg/redis"
	"payment-system/pkg/server"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("Failed to load configuration:", err)
	}

	// Initialize logger
	logger := logger.New(cfg.Logging.Level, cfg.Logging.Format)
	defer logger.Sync()

	logger.Info("Starting payment system", 
		zap.String("version", "1.0.0"),
		zap.String("environment", os.Getenv("ENVIRONMENT")))

	// Inicializar base de datos
	db, err := database.New(cfg.Database)
	if err != nil {
		logger.Fatal("Failed to initialize database", zap.Error(err))
	}
	defer db.Close()

	// Inicializar Redis
	redisClient, err := redis.New(cfg.Redis)
	if err != nil {
		logger.Fatal("Failed to initialize Redis", zap.Error(err))
	}
	defer redisClient.Close()

	// Inicializar event publisher y subscriber
	eventPublisher := events.NewKafkaPublisher(cfg.Kafka.Brokers, cfg.Kafka.Topic, logger)
	defer eventPublisher.Close()

	eventSubscriber := events.NewKafkaSubscriber(cfg.Kafka.Brokers, cfg.Kafka.Topic, cfg.Kafka.ConsumerGroup, logger)
	defer eventSubscriber.Stop()

	// Inicializar repositorios
	paymentRepo := database.NewPaymentRepository(db)
	walletRepo := database.NewWalletRepository(db)
	metricsRepo := database.NewMetricsRepository(db)
	sagaRepo := database.NewSagaRepository(db)

	// Inicializar servicios
	metricsService := metrics.NewService(metricsRepo, eventPublisher, logger)
	walletService := wallet.NewService(walletRepo, eventPublisher, logger)
	gatewayService := gateway.NewService(
		cfg.Gateway.BaseURL,
		cfg.Gateway.APIKey,
		gateway.GatewayProvider(cfg.Gateway.Provider),
		eventPublisher,
		logger,
	)
	paymentService := payment.NewService(paymentRepo, walletService, gatewayService, eventPublisher, logger)

	// Inicializar saga orchestrator
	sagaOrchestrator := saga.NewOrchestrator(sagaRepo, eventPublisher, eventSubscriber, logger)

	// Registrar manejadores de eventos para métricas
	registerMetricsEventHandlers(eventSubscriber, metricsService, logger)

	// Inicializar servidor HTTP
	router := gin.New()
	router.Use(gin.Logger())
	router.Use(gin.Recovery())

	// Configurar rutas
	setupRoutes(router, paymentService, walletService, metricsService, sagaOrchestrator)

	// Servidor de métricas separado
	if cfg.Metrics.Enabled {
		go startMetricsServer(cfg.Metrics.Port, cfg.Metrics.Path, logger)
		go startMetricsChecker(metricsService, cfg.Metrics.AlertCheckInterval, logger)
	}

	// Inicializar servidor principal
	srv := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	// Iniciar servicios en goroutines
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Iniciar event subscriber
	go func() {
		if err := eventSubscriber.Start(ctx); err != nil {
			logger.Error("Event subscriber failed", zap.Error(err))
		}
	}()

	// Iniciar saga orchestrator
	go func() {
		if err := sagaOrchestrator.Start(ctx); err != nil {
			logger.Error("Saga orchestrator failed", zap.Error(err))
		}
	}()

	// Iniciar servidor HTTP
	go func() {
		logger.Info("Starting HTTP server", 
			zap.String("address", srv.Addr))
		
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Failed to start server", zap.Error(err))
		}
	}()

	// Esperar señal de interrupción
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down server...")

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("Server forced to shutdown", zap.Error(err))
	}

	logger.Info("Server exited")
}

// setupRoutes configura las rutas del API
func setupRoutes(
	router *gin.Engine,
	paymentService *payment.Service,
	walletService *wallet.Service,
	metricsService *metrics.Service,
	sagaOrchestrator *saga.Orchestrator,
) {
	api := router.Group("/api/v1")

	// Rutas de salud
	router.GET("/health", server.HealthHandler)
	router.GET("/ready", server.ReadinessHandler)
	router.GET("/live", server.LivenessHandler)
	router.GET("/metrics", server.MetricsHandler)

	// Rutas de pagos
	payments := api.Group("/payments")
	payments.POST("/", server.CreatePaymentHandler(paymentService))
	payments.GET("/:id", server.GetPaymentHandler(paymentService))

	// Rutas de billetera
	wallets := api.Group("/wallets")
	wallets.GET("/:user_id/:currency", server.GetWalletHandler(walletService))
	wallets.GET("/:user_id/:currency/transactions", server.GetTransactionHistoryHandler(walletService))

	// Rutas de métricas
	metricsAPI := api.Group("/metrics")
	metricsAPI.GET("/", server.GetMetricsHandler(metricsService))
	metricsAPI.GET("/alerts", server.GetAlertsHandler(metricsService))
	metricsAPI.POST("/alerts/rules", server.CreateAlertRuleHandler(metricsService))

	// Rutas de sagas
	sagas := api.Group("/sagas")
	sagas.GET("/:id", server.GetSagaHandler(sagaOrchestrator))
}

// registerMetricsEventHandlers registra manejadores de eventos para métricas
func registerMetricsEventHandlers(subscriber events.Subscriber, metricsService *metrics.Service, logger *zap.Logger) {
	ctx := context.Background()

	logger.Info("Registering metrics event handlers")

	// Manejar eventos de pago iniciado
	subscriber.Subscribe(ctx, events.PaymentInitiated, func(ctx context.Context, event interface{}) error {
		if paymentEvent, ok := event.(events.PaymentInitiatedEvent); ok {
			logger.Debug("Processing PaymentInitiated event", 
				zap.String("payment_id", paymentEvent.AggregateID.String()),
				zap.String("user_id", paymentEvent.UserID.String()))
			metricsService.RecordPaymentInitiated(ctx, paymentEvent.AggregateID, paymentEvent.UserID, paymentEvent.Amount)
		}
		return nil
	})

	// Manejar eventos de pago completado
	subscriber.Subscribe(ctx, events.PaymentCompleted, func(ctx context.Context, event interface{}) error {
		if paymentEvent, ok := event.(events.PaymentCompletedEvent); ok {
			duration := time.Since(paymentEvent.ProcessedAt)
			metricsService.RecordPaymentCompleted(ctx, paymentEvent.AggregateID, duration)
		}
		return nil
	})

	// Manejar eventos de pago fallido
	subscriber.Subscribe(ctx, events.PaymentFailed, func(ctx context.Context, event interface{}) error {
		if paymentEvent, ok := event.(events.PaymentFailedEvent); ok {
			metricsService.RecordPaymentFailed(ctx, paymentEvent.AggregateID, paymentEvent.ErrorCode)
		}
		return nil
	})

	// Manejar eventos de billetera
	subscriber.Subscribe(ctx, events.WalletDeducted, func(ctx context.Context, event interface{}) error {
		if walletEvent, ok := event.(events.WalletDeductedEvent); ok {
			metricsService.RecordWalletBalance(ctx, walletEvent.UserID, walletEvent.Currency, walletEvent.NewBalance)
		}
		return nil
	})

	subscriber.Subscribe(ctx, events.WalletRefunded, func(ctx context.Context, event interface{}) error {
		if walletEvent, ok := event.(events.WalletRefundedEvent); ok {
			metricsService.RecordWalletBalance(ctx, walletEvent.UserID, walletEvent.Currency, walletEvent.NewBalance)
		}
		return nil
	})

	// Manejar eventos de pasarela
	subscriber.Subscribe(ctx, events.GatewayResponseReceived, func(ctx context.Context, event interface{}) error {
		if gatewayEvent, ok := event.(events.GatewayResponseReceivedEvent); ok {
			metricsService.RecordGatewayResponseTime(ctx, "gateway", gatewayEvent.ProcessingTime)
		}
		return nil
	})
}

// startMetricsServer inicia el servidor de métricas de Prometheus
func startMetricsServer(port int, path string, logger *zap.Logger) {
	mux := http.NewServeMux()
	mux.Handle(path, promhttp.Handler())

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	logger.Info("Starting metrics server", zap.Int("port", port), zap.String("path", path))

	if err := srv.ListenAndServe(); err != nil {
		logger.Error("Metrics server failed", zap.Error(err))
	}
}

// startMetricsChecker inicia el verificador de alertas de métricas
func startMetricsChecker(metricsService *metrics.Service, interval time.Duration, logger *zap.Logger) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	logger.Info("Starting metrics alert checker", zap.Duration("interval", interval))

	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		
		if err := metricsService.CheckAlerts(ctx); err != nil {
			logger.Error("Failed to check alerts", zap.Error(err))
		}
		
		cancel()
	}
}
