# Estrategia Integral de Manejo de Errores

## Índice
1. [Escenarios de Falla Identificados](#escenarios-de-falla-identificados)
2. [Reintentos y Backoff Exponencial](#reintentos-y-backoff-exponencial)
3. [Dead Letter Queue (DLQ)](#dead-letter-queue-dlq)
4. [Transacciones Compensatorias](#transacciones-compensatorias)
5. [Circuit Breaker](#circuit-breaker)
6. [Monitoreo y Alertas](#monitoreo-y-alertas)

---

## Escenarios de Falla Identificados

### 1. Fallos de Validación de Negocio

#### Saldo Insuficiente en Billetera
```yaml
Escenario: Usuario intenta pago > saldo disponible
Detección: Validación en Wallet Service antes de deducción
Respuesta: Inmediata, sin reintentos
Evento: WalletDeductionFailed
Acción: Marcar pago como fallido, notificar usuario
```

#### Límites de Transacción Excedidos
```yaml
Escenario: Pago excede límites diarios/mensuales
Detección: Validación en Payment Service
Respuesta: Rechazo inmediato
Evento: PaymentValidationFailed
Acción: Respuesta HTTP 400, log para análisis
```

### 2. Fallos de Comunicación Externa

#### Timeout de Pasarela de Pago
```yaml
Escenario: Pasarela no responde en tiempo esperado
Detección: Timeout HTTP (30s)
Respuesta: Reintento con backoff exponencial
Evento: GatewayTimeoutOccurred
Acción: Hasta 3 reintentos, luego compensación
```

#### Error HTTP de Pasarela
```yaml
Escenario: Pasarela retorna 5xx o error de red
Detección: Status code >= 500 o connection error
Respuesta: Reintento según tipo de error
Evento: GatewayErrorReceived
Acción: Reintentos para 502/503/504, fallo para otros
```

### 3. Fallos de Infraestructura

#### Caída del Servicio de Mensajería (Kafka)
```yaml
Escenario: Kafka cluster no disponible
Detección: Connection timeout, metadata errors
Respuesta: Outbox pattern + reintentos
Evento: MessageBrokerUnavailable
Acción: Almacenar eventos en DB, publicar cuando recupere
```

#### Errores de Base de Datos
```yaml
Escenario: PostgreSQL connection/query errors
Detección: SQL errors, connection pool exhausted
Respuesta: Reintentos con circuit breaker
Evento: DatabaseErrorOccurred
Acción: Reintentos para errores temporales, alerta para otros
```

### 4. Fallos de Consistencia

#### Eventos Duplicados
```yaml
Escenario: Mismo evento procesado múltiples veces
Detección: Idempotency key check
Respuesta: Ignorar duplicado
Evento: DuplicateEventDetected
Acción: Log para auditoría, continuar procesamiento
```

#### Eventos Fuera de Orden
```yaml
Escenario: Eventos llegan en orden incorrecto
Detección: Version/timestamp check
Respuesta: Reordenar o rechazar
Evento: OutOfOrderEventDetected
Acción: Buffer temporal o DLQ según criticidad
```

---

## Reintentos y Backoff Exponencial

### Configuración por Tipo de Operación

```go
type RetryConfig struct {
    MaxRetries      int
    BaseDelay       time.Duration
    MaxDelay        time.Duration
    BackoffFactor   float64
    Jitter          bool
}

var RetryConfigs = map[string]RetryConfig{
    "gateway_call": {
        MaxRetries:    3,
        BaseDelay:     100 * time.Millisecond,
        MaxDelay:      5 * time.Second,
        BackoffFactor: 2.0,
        Jitter:        true,
    },
    "database_operation": {
        MaxRetries:    5,
        BaseDelay:     50 * time.Millisecond,
        MaxDelay:      2 * time.Second,
        BackoffFactor: 1.5,
        Jitter:        true,
    },
    "kafka_publish": {
        MaxRetries:    10,
        BaseDelay:     10 * time.Millisecond,
        MaxDelay:      1 * time.Second,
        BackoffFactor: 2.0,
        Jitter:        false,
    },
}
```

### Implementación de Retry con Jitter

```go
func RetryWithBackoff(ctx context.Context, operation func() error, config RetryConfig) error {
    var lastErr error
    
    for attempt := 0; attempt <= config.MaxRetries; attempt++ {
        if attempt > 0 {
            delay := calculateDelay(attempt, config)
            select {
            case <-time.After(delay):
            case <-ctx.Done():
                return ctx.Err()
            }
        }
        
        if err := operation(); err == nil {
            return nil
        } else {
            lastErr = err
            
            // No reintentar errores permanentes
            if !isRetryableError(err) {
                return err
            }
        }
    }
    
    return fmt.Errorf("operation failed after %d attempts: %w", 
        config.MaxRetries, lastErr)
}

func calculateDelay(attempt int, config RetryConfig) time.Duration {
    delay := time.Duration(float64(config.BaseDelay) * 
        math.Pow(config.BackoffFactor, float64(attempt-1)))
    
    if delay > config.MaxDelay {
        delay = config.MaxDelay
    }
    
    if config.Jitter {
        jitter := time.Duration(rand.Float64() * float64(delay) * 0.1)
        delay += jitter
    }
    
    return delay
}
```

### Clasificación de Errores

```go
func isRetryableError(err error) bool {
    switch {
    case isNetworkError(err):
        return true
    case isTimeoutError(err):
        return true
    case isDatabaseConnectionError(err):
        return true
    case isHTTP5xxError(err):
        return true
    case isKafkaTemporaryError(err):
        return true
    default:
        return false
    }
}
```

---

## Dead Letter Queue (DLQ)

### Configuración de DLQ por Dominio

```yaml
DLQ_Topics:
  payment-events-dlq:
    retention_ms: 2592000000  # 30 días
    partitions: 3
    replication_factor: 3
    
  wallet-events-dlq:
    retention_ms: 5184000000  # 60 días (más crítico)
    partitions: 2
    replication_factor: 3
    
  gateway-events-dlq:
    retention_ms: 1209600000  # 14 días
    partitions: 2
    replication_factor: 3
```

### Estructura de Mensaje DLQ

```go
type DLQMessage struct {
    OriginalTopic     string                 `json:"original_topic"`
    OriginalPartition int32                  `json:"original_partition"`
    OriginalOffset    int64                  `json:"original_offset"`
    OriginalMessage   []byte                 `json:"original_message"`
    ErrorDetails      ErrorDetails           `json:"error_details"`
    RetryHistory      []RetryAttempt         `json:"retry_history"`
    CreatedAt         time.Time              `json:"created_at"`
    Priority          string                 `json:"priority"`
}

type ErrorDetails struct {
    ErrorType    string `json:"error_type"`
    ErrorMessage string `json:"error_message"`
    StackTrace   string `json:"stack_trace"`
    ServiceName  string `json:"service_name"`
    Version      string `json:"version"`
}

type RetryAttempt struct {
    AttemptNumber int       `json:"attempt_number"`
    Timestamp     time.Time `json:"timestamp"`
    ErrorMessage  string    `json:"error_message"`
}
```

### Procesamiento de DLQ

```go
func (p *DLQProcessor) ProcessDLQMessage(msg DLQMessage) error {
    // 1. Analizar tipo de error
    errorAnalysis := p.analyzeError(msg.ErrorDetails)
    
    // 2. Determinar estrategia de recuperación
    switch errorAnalysis.Category {
    case "transient":
        return p.retryOriginalOperation(msg)
    case "configuration":
        return p.scheduleManualReview(msg)
    case "data_corruption":
        return p.quarantineMessage(msg)
    case "business_logic":
        return p.createCompensationEvent(msg)
    }
    
    return nil
}
```

### Dashboard de DLQ

```yaml
DLQ_Monitoring:
  metrics:
    - dlq_message_count_by_topic
    - dlq_message_age_histogram
    - dlq_processing_rate
    - dlq_retry_success_rate
    
  alerts:
    - DLQ message count > 100
    - DLQ message age > 24 hours
    - DLQ processing failure rate > 10%
```

---

## Transacciones Compensatorias

### Saga de Compensación para Pagos

```go
type PaymentCompensationSaga struct {
    PaymentID     string
    Steps         []CompensationStep
    CurrentStep   int
    Status        CompensationStatus
}

type CompensationStep struct {
    Name          string
    Service       string
    Command       CompensationCommand
    Status        StepStatus
    ExecutedAt    *time.Time
    Error         *string
}
```

### Escenarios de Compensación

#### 1. Fallo después de Deducción de Billetera

```yaml
Escenario: Billetera deducida, pero falla pasarela
Compensación:
  1. RefundWalletBalance
  2. MarkPaymentAsFailed
  3. NotifyUser
  4. UpdateMetrics

Eventos_Generados:
  - CompensationStarted
  - WalletRefunded
  - PaymentFailed
  - CompensationCompleted
```

#### 2. Fallo Parcial en Pasarela

```yaml
Escenario: Pasarela procesa pero no confirma
Compensación:
  1. QueryGatewayStatus (verificar estado real)
  2. Si procesado: ConfirmPayment
  3. Si no procesado: RefundWallet
  4. UpdatePaymentStatus

Timeout: 5 minutos para verificación
```

### Implementación de Compensación

```go
func (s *SagaOrchestrator) ExecuteCompensation(sagaID string) error {
    saga, err := s.repo.GetSaga(sagaID)
    if err != nil {
        return err
    }
    
    // Ejecutar pasos de compensación en orden inverso
    for i := len(saga.CompletedSteps) - 1; i >= 0; i-- {
        step := saga.CompletedSteps[i]
        
        compensationCmd := s.buildCompensationCommand(step)
        if err := s.executeCompensation(compensationCmd); err != nil {
            // Log error pero continuar con otros pasos
            s.logger.Error("Compensation step failed", 
                zap.String("saga_id", sagaID),
                zap.String("step", step.Name),
                zap.Error(err))
        }
    }
    
    return s.markSagaAsCompensated(sagaID)
}
```

### Compensación Idempotente

```go
func (w *WalletService) RefundBalance(cmd RefundCommand) error {
    // Verificar si ya se procesó esta compensación
    if w.isAlreadyRefunded(cmd.TransactionID) {
        return nil // Idempotente
    }
    
    return w.executeRefund(cmd)
}
```

---

## Circuit Breaker

### Configuración por Servicio Externo

```go
type CircuitBreakerConfig struct {
    MaxFailures     int
    ResetTimeout    time.Duration
    FailureRate     float64
    MinRequests     int
    HalfOpenMaxCalls int
}

var CircuitConfigs = map[string]CircuitBreakerConfig{
    "stripe_gateway": {
        MaxFailures:      5,
        ResetTimeout:     60 * time.Second,
        FailureRate:      0.5,
        MinRequests:      10,
        HalfOpenMaxCalls: 3,
    },
    "paypal_gateway": {
        MaxFailures:      3,
        ResetTimeout:     30 * time.Second,
        FailureRate:      0.6,
        MinRequests:      5,
        HalfOpenMaxCalls: 2,
    },
}
```

### Estados del Circuit Breaker

```go
type CircuitState int

const (
    StateClosed CircuitState = iota
    StateOpen
    StateHalfOpen
)

type CircuitBreaker struct {
    config       CircuitBreakerConfig
    state        CircuitState
    failures     int
    requests     int
    lastFailTime time.Time
    mutex        sync.RWMutex
}
```

### Implementación con Redis Distribuido

```go
func (cb *DistributedCircuitBreaker) Call(operation func() error) error {
    state, err := cb.getState()
    if err != nil {
        return err
    }
    
    switch state {
    case StateClosed:
        return cb.executeAndRecord(operation)
    case StateOpen:
        if cb.shouldAttemptReset() {
            return cb.attemptHalfOpen(operation)
        }
        return ErrCircuitBreakerOpen
    case StateHalfOpen:
        return cb.executeHalfOpen(operation)
    }
    
    return nil
}

func (cb *DistributedCircuitBreaker) executeAndRecord(operation func() error) error {
    err := operation()
    
    if err != nil {
        cb.recordFailure()
        if cb.shouldOpen() {
            cb.openCircuit()
        }
    } else {
        cb.recordSuccess()
    }
    
    return err
}
```

### Fallback Strategies

```go
func (g *GatewayService) ProcessPayment(payment Payment) error {
    return g.circuitBreaker.Call(func() error {
        return g.callExternalGateway(payment)
    }, func() error {
        // Fallback: queue for later processing
        return g.queueForLaterProcessing(payment)
    })
}
```

---

## Monitoreo y Alertas

### Métricas Clave de Errores

```yaml
Error_Metrics:
  - error_rate_by_service
  - error_rate_by_type
  - retry_attempts_histogram
  - dlq_message_count
  - circuit_breaker_state_changes
  - compensation_execution_time
  - saga_failure_rate

Business_Impact_Metrics:
  - payment_success_rate
  - average_payment_processing_time
  - wallet_consistency_violations
  - revenue_impact_of_failures
```

### Alertas Críticas

```yaml
Critical_Alerts:
  - Payment success rate < 99%
  - DLQ messages > 1000
  - Circuit breaker open > 5 minutes
  - Saga compensation rate > 5%
  - Database error rate > 1%

Alert_Channels:
  - PagerDuty for critical issues
  - Slack for warnings
  - Email for daily summaries
```

Esta estrategia integral de manejo de errores garantiza la resiliencia y consistencia del sistema de pagos, proporcionando múltiples capas de protección y recuperación automática.
