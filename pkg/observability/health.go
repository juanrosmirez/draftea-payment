package observability

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"sync"
	"time"

	"go.uber.org/zap"
)

// HealthStatus representa el estado de salud de un componente
type HealthStatus string

const (
	HealthStatusHealthy   HealthStatus = "healthy"
	HealthStatusUnhealthy HealthStatus = "unhealthy"
	HealthStatusDegraded  HealthStatus = "degraded"
)

// HealthCheck define la interfaz para verificaciones de salud
type HealthCheck interface {
	Name() string
	Check(ctx context.Context) HealthCheckResult
}

// HealthCheckResult representa el resultado de una verificación de salud
type HealthCheckResult struct {
	Name      string                 `json:"name"`
	Status    HealthStatus           `json:"status"`
	Message   string                 `json:"message,omitempty"`
	Duration  time.Duration          `json:"duration"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
}

// HealthChecker gestiona múltiples verificaciones de salud
type HealthChecker struct {
	checks   map[string]HealthCheck
	mu       sync.RWMutex
	logger   StructuredLogger
	timeout  time.Duration
}

// NewHealthChecker crea un nuevo verificador de salud
func NewHealthChecker(logger StructuredLogger, timeout time.Duration) *HealthChecker {
	return &HealthChecker{
		checks:  make(map[string]HealthCheck),
		logger:  logger,
		timeout: timeout,
	}
}

// RegisterCheck registra una nueva verificación de salud
func (hc *HealthChecker) RegisterCheck(check HealthCheck) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	
	hc.checks[check.Name()] = check
	hc.logger.Info("Health check registered",
		zap.String("check_name", check.Name()),
	)
}

// CheckAll ejecuta todas las verificaciones de salud
func (hc *HealthChecker) CheckAll(ctx context.Context) map[string]HealthCheckResult {
	hc.mu.RLock()
	checks := make(map[string]HealthCheck, len(hc.checks))
	for name, check := range hc.checks {
		checks[name] = check
	}
	hc.mu.RUnlock()
	
	results := make(map[string]HealthCheckResult)
	var wg sync.WaitGroup
	var mu sync.Mutex
	
	for name, check := range checks {
		wg.Add(1)
		go func(name string, check HealthCheck) {
			defer wg.Done()
			
			// Crear contexto con timeout
			checkCtx, cancel := context.WithTimeout(ctx, hc.timeout)
			defer cancel()
			
			start := time.Now()
			result := check.Check(checkCtx)
			result.Duration = time.Since(start)
			result.Timestamp = time.Now().UTC()
			
			mu.Lock()
			results[name] = result
			mu.Unlock()
		}(name, check)
	}
	
	wg.Wait()
	return results
}

// GetOverallStatus determina el estado general del sistema
func (hc *HealthChecker) GetOverallStatus(results map[string]HealthCheckResult) HealthStatus {
	if len(results) == 0 {
		return HealthStatusHealthy
	}
	
	hasUnhealthy := false
	hasDegraded := false
	
	for _, result := range results {
		switch result.Status {
		case HealthStatusUnhealthy:
			hasUnhealthy = true
		case HealthStatusDegraded:
			hasDegraded = true
		}
	}
	
	if hasUnhealthy {
		return HealthStatusUnhealthy
	}
	if hasDegraded {
		return HealthStatusDegraded
	}
	
	return HealthStatusHealthy
}

// DatabaseHealthCheck verifica la conectividad de la base de datos
type DatabaseHealthCheck struct {
	name string
	db   *sql.DB
}

// NewDatabaseHealthCheck crea una verificación de salud para base de datos
func NewDatabaseHealthCheck(name string, db *sql.DB) *DatabaseHealthCheck {
	return &DatabaseHealthCheck{
		name: name,
		db:   db,
	}
}

func (dhc *DatabaseHealthCheck) Name() string {
	return dhc.name
}

func (dhc *DatabaseHealthCheck) Check(ctx context.Context) HealthCheckResult {
	result := HealthCheckResult{
		Name:     dhc.name,
		Metadata: make(map[string]interface{}),
	}
	
	// Verificar conexión con ping
	if err := dhc.db.PingContext(ctx); err != nil {
		result.Status = HealthStatusUnhealthy
		result.Message = fmt.Sprintf("Database ping failed: %v", err)
		return result
	}
	
	// Verificar estadísticas de conexión
	stats := dhc.db.Stats()
	result.Metadata["open_connections"] = stats.OpenConnections
	result.Metadata["in_use"] = stats.InUse
	result.Metadata["idle"] = stats.Idle
	result.Metadata["wait_count"] = stats.WaitCount
	result.Metadata["wait_duration"] = stats.WaitDuration.String()
	
	// Verificar si hay demasiadas conexiones en espera
	if stats.WaitCount > 10 {
		result.Status = HealthStatusDegraded
		result.Message = "High connection wait count detected"
		return result
	}
	
	// Ejecutar una consulta simple
	var count int
	err := dhc.db.QueryRowContext(ctx, "SELECT 1").Scan(&count)
	if err != nil {
		result.Status = HealthStatusUnhealthy
		result.Message = fmt.Sprintf("Database query failed: %v", err)
		return result
	}
	
	result.Status = HealthStatusHealthy
	result.Message = "Database connection healthy"
	return result
}

// EventStoreHealthCheck verifica la salud del Event Store
type EventStoreHealthCheck struct {
	name       string
	eventStore EventStoreInterface
}

// NewEventStoreHealthCheck crea una verificación de salud para Event Store
func NewEventStoreHealthCheck(name string, eventStore EventStoreInterface) *EventStoreHealthCheck {
	return &EventStoreHealthCheck{
		name:       name,
		eventStore: eventStore,
	}
}

func (eshc *EventStoreHealthCheck) Name() string {
	return eshc.name
}

func (eshc *EventStoreHealthCheck) Check(ctx context.Context) HealthCheckResult {
	result := HealthCheckResult{
		Name:     eshc.name,
		Metadata: make(map[string]interface{}),
	}
	
	// Intentar obtener eventos recientes
	filter := EventFilter{
		EventTypes: []string{}, // Obtener cualquier tipo de evento
	}
	
	events, err := eshc.eventStore.GetEvents(ctx, filter)
	if err != nil {
		result.Status = HealthStatusUnhealthy
		result.Message = fmt.Sprintf("Event Store query failed: %v", err)
		return result
	}
	
	result.Metadata["total_events"] = len(events)
	
	// Verificar si hay eventos recientes (últimos 5 minutos)
	recentEvents := 0
	fiveMinutesAgo := time.Now().Add(-5 * time.Minute)
	
	for _, event := range events {
		if event.CreatedAt.After(fiveMinutesAgo) {
			recentEvents++
		}
	}
	
	result.Metadata["recent_events"] = recentEvents
	
	if len(events) == 0 {
		result.Status = HealthStatusDegraded
		result.Message = "No events found in Event Store"
	} else {
		result.Status = HealthStatusHealthy
		result.Message = "Event Store is accessible"
	}
	
	return result
}

// RedisHealthCheck verifica la conectividad con Redis
type RedisHealthCheck struct {
	name   string
	client RedisClient
}

// RedisClient define la interfaz mínima para cliente Redis
type RedisClient interface {
	Ping(ctx context.Context) error
	Info(ctx context.Context) (string, error)
}

// NewRedisHealthCheck crea una verificación de salud para Redis
func NewRedisHealthCheck(name string, client RedisClient) *RedisHealthCheck {
	return &RedisHealthCheck{
		name:   name,
		client: client,
	}
}

func (rhc *RedisHealthCheck) Name() string {
	return rhc.name
}

func (rhc *RedisHealthCheck) Check(ctx context.Context) HealthCheckResult {
	result := HealthCheckResult{
		Name:     rhc.name,
		Metadata: make(map[string]interface{}),
	}
	
	// Verificar conexión con ping
	if err := rhc.client.Ping(ctx); err != nil {
		result.Status = HealthStatusUnhealthy
		result.Message = fmt.Sprintf("Redis ping failed: %v", err)
		return result
	}
	
	// Obtener información del servidor
	info, err := rhc.client.Info(ctx)
	if err != nil {
		result.Status = HealthStatusDegraded
		result.Message = fmt.Sprintf("Redis info command failed: %v", err)
		return result
	}
	
	result.Metadata["redis_info_length"] = len(info)
	result.Status = HealthStatusHealthy
	result.Message = "Redis connection healthy"
	
	return result
}

// ExternalServiceHealthCheck verifica servicios externos via HTTP
type ExternalServiceHealthCheck struct {
	name     string
	url      string
	client   *http.Client
	expected int // Status code esperado
}

// NewExternalServiceHealthCheck crea una verificación para servicios externos
func NewExternalServiceHealthCheck(name, url string, expectedStatus int) *ExternalServiceHealthCheck {
	return &ExternalServiceHealthCheck{
		name:     name,
		url:      url,
		expected: expectedStatus,
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

func (eshc *ExternalServiceHealthCheck) Name() string {
	return eshc.name
}

func (eshc *ExternalServiceHealthCheck) Check(ctx context.Context) HealthCheckResult {
	result := HealthCheckResult{
		Name:     eshc.name,
		Metadata: make(map[string]interface{}),
	}
	
	req, err := http.NewRequestWithContext(ctx, "GET", eshc.url, nil)
	if err != nil {
		result.Status = HealthStatusUnhealthy
		result.Message = fmt.Sprintf("Failed to create request: %v", err)
		return result
	}
	
	resp, err := eshc.client.Do(req)
	if err != nil {
		result.Status = HealthStatusUnhealthy
		result.Message = fmt.Sprintf("HTTP request failed: %v", err)
		return result
	}
	defer resp.Body.Close()
	
	result.Metadata["status_code"] = resp.StatusCode
	result.Metadata["expected_status"] = eshc.expected
	
	if resp.StatusCode != eshc.expected {
		result.Status = HealthStatusUnhealthy
		result.Message = fmt.Sprintf("Unexpected status code: got %d, expected %d", resp.StatusCode, eshc.expected)
		return result
	}
	
	result.Status = HealthStatusHealthy
	result.Message = "External service is responding"
	return result
}

// MemoryHealthCheck verifica el uso de memoria
type MemoryHealthCheck struct {
	name            string
	maxMemoryMB     int64
	warningThreshold float64 // Porcentaje para estado degraded
}

// NewMemoryHealthCheck crea una verificación de uso de memoria
func NewMemoryHealthCheck(name string, maxMemoryMB int64, warningThreshold float64) *MemoryHealthCheck {
	return &MemoryHealthCheck{
		name:            name,
		maxMemoryMB:     maxMemoryMB,
		warningThreshold: warningThreshold,
	}
}

func (mhc *MemoryHealthCheck) Name() string {
	return mhc.name
}

func (mhc *MemoryHealthCheck) Check(ctx context.Context) HealthCheckResult {
	result := HealthCheckResult{
		Name:     mhc.name,
		Metadata: make(map[string]interface{}),
	}
	
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	
	allocMB := int64(m.Alloc / 1024 / 1024)
	sysMB := int64(m.Sys / 1024 / 1024)
	
	result.Metadata["alloc_mb"] = allocMB
	result.Metadata["sys_mb"] = sysMB
	result.Metadata["max_mb"] = mhc.maxMemoryMB
	result.Metadata["gc_cycles"] = m.NumGC
	
	usagePercent := float64(allocMB) / float64(mhc.maxMemoryMB) * 100
	result.Metadata["usage_percent"] = usagePercent
	
	if allocMB > mhc.maxMemoryMB {
		result.Status = HealthStatusUnhealthy
		result.Message = fmt.Sprintf("Memory usage exceeded limit: %dMB > %dMB", allocMB, mhc.maxMemoryMB)
	} else if usagePercent > mhc.warningThreshold {
		result.Status = HealthStatusDegraded
		result.Message = fmt.Sprintf("Memory usage high: %.1f%%", usagePercent)
	} else {
		result.Status = HealthStatusHealthy
		result.Message = fmt.Sprintf("Memory usage normal: %dMB (%.1f%%)", allocMB, usagePercent)
	}
	
	return result
}

// HealthResponse representa la respuesta del endpoint de salud
type HealthResponse struct {
	Status    HealthStatus                   `json:"status"`
	Timestamp time.Time                      `json:"timestamp"`
	Duration  time.Duration                  `json:"duration"`
	Checks    map[string]HealthCheckResult   `json:"checks"`
	Metadata  map[string]interface{}         `json:"metadata,omitempty"`
}

// HealthHandler crea handlers HTTP para endpoints de salud
type HealthHandler struct {
	checker *HealthChecker
	logger  StructuredLogger
}

// NewHealthHandler crea un nuevo handler de salud
func NewHealthHandler(checker *HealthChecker, logger StructuredLogger) *HealthHandler {
	return &HealthHandler{
		checker: checker,
		logger:  logger,
	}
}

// HealthzHandler maneja el endpoint /healthz (verificación básica)
func (hh *HealthHandler) HealthzHandler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx := r.Context()
	
	// Ejecutar verificaciones críticas solamente
	results := hh.checker.CheckAll(ctx)
	overallStatus := hh.checker.GetOverallStatus(results)
	
	response := HealthResponse{
		Status:    overallStatus,
		Timestamp: time.Now().UTC(),
		Duration:  time.Since(start),
		Checks:    results,
		Metadata: map[string]interface{}{
			"service": "payment-system",
			"version": "1.0.0",
		},
	}
	
	// Determinar status code HTTP
	var statusCode int
	switch overallStatus {
	case HealthStatusHealthy:
		statusCode = http.StatusOK
	case HealthStatusDegraded:
		statusCode = http.StatusOK // 200 pero con advertencias
	case HealthStatusUnhealthy:
		statusCode = http.StatusServiceUnavailable
	}
	
	// Log resultado
	hh.logger.WithContext(ctx).Info("Health check completed",
		zap.String("overall_status", string(overallStatus)),
		zap.Int("status_code", statusCode),
		zap.Duration("duration", response.Duration),
		zap.Int("checks_count", len(results)),
	)
	
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	
	if err := json.NewEncoder(w).Encode(response); err != nil {
		hh.logger.Error("Failed to encode health response",
			zap.Error(err),
		)
	}
}

// ReadyHandler maneja el endpoint /ready (listo para recibir tráfico)
func (hh *HealthHandler) ReadyHandler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx := r.Context()
	
	// Para readiness, verificar solo dependencias críticas
	results := hh.checker.CheckAll(ctx)
	
	// Filtrar solo verificaciones críticas para readiness
	criticalChecks := make(map[string]HealthCheckResult)
	for name, result := range results {
		// Solo incluir verificaciones críticas (DB, Event Store)
		if name == "database" || name == "eventstore" {
			criticalChecks[name] = result
		}
	}
	
	overallStatus := hh.checker.GetOverallStatus(criticalChecks)
	
	response := HealthResponse{
		Status:    overallStatus,
		Timestamp: time.Now().UTC(),
		Duration:  time.Since(start),
		Checks:    criticalChecks,
		Metadata: map[string]interface{}{
			"service": "payment-system",
			"check_type": "readiness",
		},
	}
	
	// Para readiness, solo healthy es aceptable
	var statusCode int
	if overallStatus == HealthStatusHealthy {
		statusCode = http.StatusOK
	} else {
		statusCode = http.StatusServiceUnavailable
	}
	
	hh.logger.WithContext(ctx).Info("Readiness check completed",
		zap.String("overall_status", string(overallStatus)),
		zap.Int("status_code", statusCode),
		zap.Duration("duration", response.Duration),
	)
	
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	
	if err := json.NewEncoder(w).Encode(response); err != nil {
		hh.logger.Error("Failed to encode readiness response",
			zap.Error(err),
		)
	}
}

// LiveHandler maneja el endpoint /live (proceso vivo)
func (hh *HealthHandler) LiveHandler(w http.ResponseWriter, r *http.Request) {
	// Liveness es simple: si podemos responder, estamos vivos
	response := map[string]interface{}{
		"status":    "alive",
		"timestamp": time.Now().UTC(),
		"service":   "payment-system",
	}
	
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	
	if err := json.NewEncoder(w).Encode(response); err != nil {
		hh.logger.Error("Failed to encode liveness response",
			zap.Error(err),
		)
	}
}

// SetupHealthChecks configura las verificaciones de salud para el sistema de pagos
func SetupHealthChecks(
	db *sql.DB,
	eventStore EventStoreInterface,
	redisClient RedisClient,
	logger StructuredLogger,
) *HealthChecker {
	checker := NewHealthChecker(logger, 5*time.Second)
	
	// Verificación de base de datos
	if db != nil {
		checker.RegisterCheck(NewDatabaseHealthCheck("database", db))
	}
	
	// Verificación de Event Store
	if eventStore != nil {
		checker.RegisterCheck(NewEventStoreHealthCheck("eventstore", eventStore))
	}
	
	// Verificación de Redis
	if redisClient != nil {
		checker.RegisterCheck(NewRedisHealthCheck("redis", redisClient))
	}
	
	// Verificación de memoria
	checker.RegisterCheck(NewMemoryHealthCheck("memory", 512, 80.0)) // 512MB max, 80% warning
	
	// Verificaciones de servicios externos (ejemplo)
	checker.RegisterCheck(NewExternalServiceHealthCheck(
		"payment_gateway", 
		"https://api.stripe.com/healthcheck", 
		200,
	))
	
	return checker
}

