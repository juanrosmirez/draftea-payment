# Event Store Implementation

Este paquete proporciona una implementación completa de Event Store para el sistema de procesamiento de pagos, siguiendo los patrones de Event Sourcing y CQRS.

## Características Principales

### ✅ Funcionalidades Implementadas

- **Interfaz EventStore completa** con operaciones básicas y avanzadas
- **Implementación en memoria** para testing y desarrollo
- **Implementación PostgreSQL** para producción con persistencia durable
- **Manejo de concurrencia** con optimistic locking
- **Snapshots** para optimización de performance
- **Suscripciones en tiempo real** usando LISTEN/NOTIFY (PostgreSQL)
- **Reconstrucción de agregados** desde eventos
- **Filtrado avanzado** de eventos
- **Atomicidad** en escritura de eventos múltiples

## Arquitectura del Event Store

### Estructura de Eventos

```go
type Event struct {
    ID               string          // ID único del evento
    AggregateID      string          // ID del agregado
    AggregateType    string          // Tipo de agregado (payment, wallet)
    EventType        string          // Tipo de evento (PaymentInitiated, etc.)
    AggregateVersion int64           // Versión del agregado
    SequenceNumber   int64           // Número de secuencia global
    EventData        json.RawMessage // Datos del evento
    Metadata         json.RawMessage // Metadatos adicionales
    CreatedAt        time.Time       // Timestamp de creación
    CausedBy         string          // Usuario que causó el evento
    CorrelationID    string          // ID de correlación
}
```

### Interfaz EventStore

```go
type EventStore interface {
    // Operaciones básicas
    AppendEvent(ctx context.Context, event *Event) error
    AppendEvents(ctx context.Context, events []*Event) error
    
    // Consultas
    GetEvents(ctx context.Context, filter EventFilter) ([]*Event, error)
    GetEventsByAggregate(ctx context.Context, aggregateID string, fromVersion int64) ([]*Event, error)
    GetEventsByAggregateType(ctx context.Context, aggregateType string, filter EventFilter) ([]*Event, error)
    
    // Metadatos
    GetLastSequenceNumber(ctx context.Context) (int64, error)
    GetAggregateVersion(ctx context.Context, aggregateID string) (int64, error)
    
    // Snapshots
    CreateSnapshot(ctx context.Context, aggregateID, aggregateType string, version int64, data json.RawMessage) error
    GetSnapshot(ctx context.Context, aggregateID string) (*Snapshot, error)
    
    // Suscripciones
    Subscribe(ctx context.Context, filter EventFilter) (<-chan *Event, error)
    
    // Limpieza
    Close() error
}
```

## Implementaciones

### 1. Memory Event Store

**Uso**: Testing, desarrollo, prototipado

**Características**:
- Almacenamiento en memoria con slices y maps
- Thread-safe con sync.RWMutex
- Suscripciones en tiempo real con canales
- Optimistic locking para prevenir conflictos de concurrencia

```go
store := NewMemoryEventStore()
defer store.Close()
```

### 2. PostgreSQL Event Store

**Uso**: Producción, persistencia durable

**Características**:
- Persistencia en PostgreSQL con transacciones ACID
- Optimistic locking con constraints únicos
- LISTEN/NOTIFY para suscripciones en tiempo real
- Índices optimizados para consultas eficientes
- Pool de conexiones configurable

```go
config := PostgresConfig{
    DSN: "postgres://user:pass@localhost/eventstore",
    EnableOptimisticLocking: true,
    EnableNotifications: true,
    NotificationChannel: "events",
}

store, err := NewPostgresEventStore(config)
if err != nil {
    log.Fatal(err)
}
defer store.Close()
```

## Manejo de Concurrencia y Atomicidad

### Optimistic Locking

El Event Store implementa optimistic locking para prevenir conflictos de concurrencia:

```go
// Cada evento tiene una versión de agregado
event.AggregateVersion = currentVersion + 1

// Si dos operaciones intentan escribir la misma versión:
// - La primera succeede
// - La segunda falla con "concurrency conflict"
```

### Transacciones Atómicas

**PostgreSQL**: Usa transacciones con nivel de aislamiento `SERIALIZABLE`
```sql
BEGIN;
-- Obtener versión actual con FOR UPDATE
-- Insertar eventos secuencialmente
-- Verificar constraints de versión
COMMIT;
```

**Memory Store**: Usa mutex para garantizar atomicidad
```go
m.mu.Lock()
defer m.mu.Unlock()
// Operaciones atómicas en memoria
```

### Manejo de Conflictos

```go
// Patrón recomendado para manejar conflictos
for retries := 0; retries < maxRetries; retries++ {
    // Cargar agregado con versión actual
    aggregate, err := repo.Load(ctx, aggregateID, factory)
    if err != nil {
        return err
    }
    
    // Aplicar cambios de negocio
    err = aggregate.ProcessBusinessLogic()
    if err != nil {
        return err
    }
    
    // Intentar guardar
    err = repo.Save(ctx, aggregate, userID, correlationID)
    if err != nil && isConcurrencyError(err) {
        // Reintentar con backoff exponencial
        time.Sleep(time.Duration(retries) * 100 * time.Millisecond)
        continue
    }
    
    return err // Éxito o error no relacionado con concurrencia
}
```

## Reconstrucción de Agregados

### AggregateRepository

Maneja la persistencia y reconstrucción de agregados:

```go
repo := NewAggregateRepository(eventStore, 10) // Snapshot cada 10 eventos

// Guardar agregado
err := repo.Save(ctx, aggregate, userID, correlationID)

// Cargar agregado (reconstruir desde eventos)
aggregate, err := repo.Load(ctx, aggregateID, func() Aggregate {
    return &PaymentAggregate{}
})

// Cargar versión específica
aggregate, err := repo.LoadVersion(ctx, aggregateID, version, factory)
```

### Patrón de Aplicación de Eventos

Los agregados implementan la interfaz `Aggregate`:

```go
type PaymentAggregate struct {
    BaseAggregate
    // ... campos de estado
}

func (p *PaymentAggregate) ApplyEvent(event *Event) error {
    switch event.EventType {
    case "PaymentInitiated":
        return p.ApplyPaymentInitiated(event)
    case "PaymentCompleted":
        return p.ApplyPaymentCompleted(event)
    default:
        return fmt.Errorf("unknown event type: %s", event.EventType)
    }
}

func (p *PaymentAggregate) ApplyPaymentInitiated(event *Event) error {
    var eventData PaymentInitiatedEventData
    if err := event.UnmarshalEventData(&eventData); err != nil {
        return err
    }
    
    // Actualizar estado del agregado
    p.Status = "initiated"
    p.Amount = eventData.Amount
    // ...
    
    return nil
}
```

## Optimización con Snapshots

### Creación Automática

```go
// Configurar frecuencia de snapshots
repo := NewAggregateRepository(eventStore, 10) // Cada 10 eventos

// Los snapshots se crean automáticamente al guardar
err := repo.Save(ctx, aggregate, userID, correlationID)
```

### Reconstrucción Optimizada

1. **Cargar snapshot más reciente** (si existe)
2. **Aplicar eventos posteriores** al snapshot
3. **Resultado**: Estado actual del agregado

```go
// Proceso interno de Load():
snapshot := eventStore.GetSnapshot(ctx, aggregateID)
if snapshot != nil {
    aggregate.LoadFromSnapshot(snapshot.Data)
    fromVersion = snapshot.AggregateVersion + 1
}

events := eventStore.GetEventsByAggregate(ctx, aggregateID, fromVersion)
for _, event := range events {
    aggregate.ApplyEvent(event)
}
```

## Ejemplos de Uso

### Ejemplo Básico

```go
ctx := context.Background()
store := NewMemoryEventStore()
defer store.Close()

repo := NewAggregateRepository(store, 5)

// Crear pago
payment := NewPaymentAggregate(paymentID, userID, 100.0, "USD", "card")
payment.InitiatePayment()

// Guardar
err := repo.Save(ctx, payment, userID, correlationID)

// Cargar
loadedPayment, err := repo.Load(ctx, paymentID, func() Aggregate {
    return &PaymentAggregate{}
})
```

### Ejemplo con Concurrencia

```go
// Operación 1
wallet1, _ := repo.Load(ctx, walletID, walletFactory)
wallet1.DeductBalance(300.0, "Purchase 1")

// Operación 2 (concurrente)
wallet2, _ := repo.Load(ctx, walletID, walletFactory)
wallet2.DeductBalance(250.0, "Purchase 2")

// Guardar operaciones
err1 := repo.Save(ctx, wallet1, userID, corr1) // ✅ Éxito
err2 := repo.Save(ctx, wallet2, userID, corr2) // ❌ Conflicto de concurrencia

// Manejar conflicto
if isConcurrencyError(err2) {
    // Recargar y reintentar
    wallet2Updated, _ := repo.Load(ctx, walletID, walletFactory)
    wallet2Updated.DeductBalance(250.0, "Purchase 2 - Retry")
    err := repo.Save(ctx, wallet2Updated, userID, corr2) // ✅ Éxito
}
```

### Suscripciones en Tiempo Real

```go
// Suscribirse a eventos de pagos
filter := EventFilter{
    AggregateType: stringPtr("payment"),
    EventTypes: []string{"PaymentCompleted", "PaymentFailed"},
}

eventChan, err := store.Subscribe(ctx, filter)
if err != nil {
    log.Fatal(err)
}

// Procesar eventos en tiempo real
for event := range eventChan {
    switch event.EventType {
    case "PaymentCompleted":
        // Enviar notificación de éxito
    case "PaymentFailed":
        // Enviar notificación de fallo
    }
}
```

## Esquema de Base de Datos (PostgreSQL)

```sql
-- Tabla principal de eventos
CREATE TABLE events (
    id VARCHAR(36) PRIMARY KEY,
    aggregate_id VARCHAR(36) NOT NULL,
    aggregate_type VARCHAR(100) NOT NULL,
    event_type VARCHAR(100) NOT NULL,
    aggregate_version BIGINT NOT NULL,
    sequence_number BIGSERIAL UNIQUE,
    event_data JSONB NOT NULL,
    metadata JSONB,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    caused_by VARCHAR(36),
    correlation_id VARCHAR(36),
    
    -- Constraint para versiones secuenciales
    CONSTRAINT unique_aggregate_version UNIQUE (aggregate_id, aggregate_version)
);

-- Tabla de snapshots
CREATE TABLE snapshots (
    aggregate_id VARCHAR(36) PRIMARY KEY,
    aggregate_type VARCHAR(100) NOT NULL,
    aggregate_version BIGINT NOT NULL,
    data JSONB NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Índices para optimización
CREATE INDEX idx_events_aggregate_id ON events (aggregate_id);
CREATE INDEX idx_events_aggregate_type ON events (aggregate_type);
CREATE INDEX idx_events_sequence_number ON events (sequence_number);
CREATE INDEX idx_events_created_at ON events (created_at);
```

## Testing

El paquete incluye tests completos:

```bash
# Ejecutar tests
go test ./pkg/eventstore/

# Tests con cobertura
go test -cover ./pkg/eventstore/

# Benchmarks
go test -bench=. ./pkg/eventstore/
```

## Consideraciones de Producción

### Performance

- **Índices optimizados** para consultas frecuentes
- **Snapshots automáticos** para agregados con muchos eventos
- **Pool de conexiones** configurable para PostgreSQL
- **Paginación** en consultas grandes

### Escalabilidad

- **Particionamiento** por aggregate_id en Kafka
- **Sharding** de base de datos por aggregate_type
- **Read replicas** para consultas de solo lectura
- **Event streaming** para proyecciones

### Monitoreo

- **Métricas** de latencia y throughput
- **Alertas** por errores de concurrencia
- **Logs estructurados** para auditoría
- **Health checks** para disponibilidad

### Backup y Recovery

- **Backup incremental** de eventos
- **Point-in-time recovery** usando sequence numbers
- **Replicación** cross-region para DR
- **Snapshots periódicos** para optimización de recovery

## Integración con el Sistema de Pagos

El Event Store se integra perfectamente con la arquitectura existente:

- **Payment Service**: Usa `PaymentAggregate` para manejar el ciclo de vida de pagos
- **Wallet Service**: Usa `WalletAggregate` para transacciones de billetera
- **Saga Orchestrator**: Consume eventos para coordinar flujos distribuidos
- **Metrics Service**: Se suscribe a eventos para métricas en tiempo real
- **Audit Service**: Registra todos los eventos para compliance

Esta implementación proporciona una base sólida para Event Sourcing en el sistema de procesamiento de pagos, garantizando consistencia, auditabilidad y escalabilidad.
