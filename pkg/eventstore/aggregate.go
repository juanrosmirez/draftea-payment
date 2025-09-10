package eventstore

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
)

// Aggregate define la interfaz que deben implementar los agregados para Event Sourcing
type Aggregate interface {
	// GetID retorna el ID único del agregado
	GetID() string
	
	// GetType retorna el tipo del agregado (payment, wallet, etc.)
	GetType() string
	
	// GetVersion retorna la versión actual del agregado
	GetVersion() int64
	
	// SetVersion establece la versión del agregado
	SetVersion(version int64)
	
	// ApplyEvent aplica un evento al agregado, modificando su estado
	ApplyEvent(event *Event) error
	
	// GetUncommittedEvents retorna eventos que aún no han sido persistidos
	GetUncommittedEvents() []*Event
	
	// MarkEventsAsCommitted marca los eventos como persistidos
	MarkEventsAsCommitted()
	
	// CreateSnapshot crea un snapshot del estado actual del agregado
	CreateSnapshot() (json.RawMessage, error)
	
	// LoadFromSnapshot carga el estado del agregado desde un snapshot
	LoadFromSnapshot(data json.RawMessage) error
}

// BaseAggregate proporciona funcionalidad común para todos los agregados
type BaseAggregate struct {
	ID                string   `json:"id"`
	Type              string   `json:"type"`
	Version           int64    `json:"version"`
	UncommittedEvents []*Event `json:"-"`
}

// GetID retorna el ID del agregado
func (b *BaseAggregate) GetID() string {
	return b.ID
}

// GetType retorna el tipo del agregado
func (b *BaseAggregate) GetType() string {
	return b.Type
}

// GetVersion retorna la versión actual del agregado
func (b *BaseAggregate) GetVersion() int64 {
	return b.Version
}

// SetVersion establece la versión del agregado
func (b *BaseAggregate) SetVersion(version int64) {
	b.Version = version
}

// GetUncommittedEvents retorna eventos no persistidos
func (b *BaseAggregate) GetUncommittedEvents() []*Event {
	return b.UncommittedEvents
}

// MarkEventsAsCommitted marca eventos como persistidos
func (b *BaseAggregate) MarkEventsAsCommitted() {
	b.UncommittedEvents = nil
}

// AddUncommittedEvent agrega un evento no persistido
func (b *BaseAggregate) AddUncommittedEvent(event *Event) {
	event.AggregateVersion = b.Version + 1
	b.UncommittedEvents = append(b.UncommittedEvents, event)
	b.Version++
}

// AggregateRepository maneja la persistencia y reconstrucción de agregados
type AggregateRepository struct {
	eventStore EventStore
	
	// Configuración para snapshots
	snapshotFrequency int64 // Crear snapshot cada N eventos
}

// NewAggregateRepository crea un nuevo repositorio de agregados
func NewAggregateRepository(eventStore EventStore, snapshotFrequency int64) *AggregateRepository {
	return &AggregateRepository{
		eventStore:        eventStore,
		snapshotFrequency: snapshotFrequency,
	}
}

// Save persiste un agregado y sus eventos no confirmados
func (r *AggregateRepository) Save(ctx context.Context, aggregate Aggregate, causedBy, correlationID string) error {
	uncommittedEvents := aggregate.GetUncommittedEvents()
	if len(uncommittedEvents) == 0 {
		return nil // No hay cambios que persistir
	}
	
	// Establecer metadatos en los eventos
	for _, event := range uncommittedEvents {
		event.AggregateID = aggregate.GetID()
		event.AggregateType = aggregate.GetType()
		event.CausedBy = causedBy
		event.CorrelationID = correlationID
	}
	
	// Persistir eventos de forma atómica
	if err := r.eventStore.AppendEvents(ctx, uncommittedEvents); err != nil {
		return fmt.Errorf("failed to save aggregate events: %w", err)
	}
	
	// Marcar eventos como confirmados
	aggregate.MarkEventsAsCommitted()
	
	// Crear snapshot si es necesario
	if r.shouldCreateSnapshot(aggregate) {
		if err := r.createSnapshot(ctx, aggregate); err != nil {
			// Log error pero no fallar la operación principal
			// En producción, esto debería loggearse apropiadamente
			fmt.Printf("Warning: failed to create snapshot for aggregate %s: %v\n", 
				aggregate.GetID(), err)
		}
	}
	
	return nil
}

// Load reconstruye un agregado desde sus eventos
func (r *AggregateRepository) Load(ctx context.Context, aggregateID string, aggregateFactory func() Aggregate) (Aggregate, error) {
	aggregate := aggregateFactory()
	
	// Intentar cargar desde snapshot primero
	snapshot, err := r.eventStore.GetSnapshot(ctx, aggregateID)
	if err != nil {
		return nil, fmt.Errorf("failed to get snapshot: %w", err)
	}
	
	var fromVersion int64 = 0
	if snapshot != nil {
		// Cargar estado desde snapshot
		if err := aggregate.LoadFromSnapshot(snapshot.Data); err != nil {
			return nil, fmt.Errorf("failed to load from snapshot: %w", err)
		}
		aggregate.SetVersion(snapshot.AggregateVersion)
		fromVersion = snapshot.AggregateVersion + 1
	}
	
	// Cargar eventos posteriores al snapshot
	events, err := r.eventStore.GetEventsByAggregate(ctx, aggregateID, fromVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to get events for aggregate: %w", err)
	}
	
	// Aplicar eventos secuencialmente para reconstruir el estado
	for _, event := range events {
		if err := aggregate.ApplyEvent(event); err != nil {
			return nil, fmt.Errorf("failed to apply event %s to aggregate: %w", 
				event.ID, err)
		}
		aggregate.SetVersion(event.AggregateVersion)
	}
	
	return aggregate, nil
}

// LoadVersion reconstruye un agregado hasta una versión específica
func (r *AggregateRepository) LoadVersion(ctx context.Context, aggregateID string, version int64, aggregateFactory func() Aggregate) (Aggregate, error) {
	aggregate := aggregateFactory()
	
	// Buscar snapshot más cercano pero no posterior a la versión solicitada
	snapshot, err := r.eventStore.GetSnapshot(ctx, aggregateID)
	if err != nil {
		return nil, fmt.Errorf("failed to get snapshot: %w", err)
	}
	
	var fromVersion int64 = 0
	if snapshot != nil && snapshot.AggregateVersion <= version {
		if err := aggregate.LoadFromSnapshot(snapshot.Data); err != nil {
			return nil, fmt.Errorf("failed to load from snapshot: %w", err)
		}
		aggregate.SetVersion(snapshot.AggregateVersion)
		fromVersion = snapshot.AggregateVersion + 1
	}
	
	// Cargar eventos hasta la versión solicitada
	events, err := r.eventStore.GetEvents(ctx, EventFilter{
		AggregateID:          &aggregateID,
		FromAggregateVersion: &fromVersion,
		ToAggregateVersion:   &version,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get events for aggregate: %w", err)
	}
	
	// Aplicar eventos
	for _, event := range events {
		if err := aggregate.ApplyEvent(event); err != nil {
			return nil, fmt.Errorf("failed to apply event %s to aggregate: %w", 
				event.ID, err)
		}
		aggregate.SetVersion(event.AggregateVersion)
	}
	
	return aggregate, nil
}

// Exists verifica si un agregado existe
func (r *AggregateRepository) Exists(ctx context.Context, aggregateID string) (bool, error) {
	version, err := r.eventStore.GetAggregateVersion(ctx, aggregateID)
	if err != nil {
		return false, fmt.Errorf("failed to check aggregate existence: %w", err)
	}
	return version > 0, nil
}

// GetVersion retorna la versión actual de un agregado sin cargarlo completamente
func (r *AggregateRepository) GetVersion(ctx context.Context, aggregateID string) (int64, error) {
	return r.eventStore.GetAggregateVersion(ctx, aggregateID)
}

// shouldCreateSnapshot determina si se debe crear un snapshot
func (r *AggregateRepository) shouldCreateSnapshot(aggregate Aggregate) bool {
	if r.snapshotFrequency <= 0 {
		return false
	}
	return aggregate.GetVersion()%r.snapshotFrequency == 0
}

// createSnapshot crea un snapshot del agregado
func (r *AggregateRepository) createSnapshot(ctx context.Context, aggregate Aggregate) error {
	data, err := aggregate.CreateSnapshot()
	if err != nil {
		return fmt.Errorf("failed to create snapshot data: %w", err)
	}
	
	return r.eventStore.CreateSnapshot(ctx, 
		aggregate.GetID(), 
		aggregate.GetType(), 
		aggregate.GetVersion(), 
		data)
}

// EventApplier es una función que aplica un evento específico a un agregado
type EventApplier func(aggregate interface{}, event *Event) error

// EventRouter maneja el enrutamiento de eventos a sus aplicadores correspondientes
type EventRouter struct {
	appliers map[string]EventApplier
}

// NewEventRouter crea un nuevo router de eventos
func NewEventRouter() *EventRouter {
	return &EventRouter{
		appliers: make(map[string]EventApplier),
	}
}

// RegisterApplier registra un aplicador para un tipo de evento específico
func (r *EventRouter) RegisterApplier(eventType string, applier EventApplier) {
	r.appliers[eventType] = applier
}

// ApplyEvent aplica un evento usando el aplicador registrado
func (r *EventRouter) ApplyEvent(aggregate interface{}, event *Event) error {
	applier, exists := r.appliers[event.EventType]
	if !exists {
		return fmt.Errorf("no applier registered for event type: %s", event.EventType)
	}
	
	return applier(aggregate, event)
}

// ReflectionEventApplier aplica eventos usando reflexión para llamar métodos
// Busca métodos con el patrón Apply{EventType}(event *Event) error
type ReflectionEventApplier struct{}

// NewReflectionEventApplier crea un nuevo aplicador basado en reflexión
func NewReflectionEventApplier() *ReflectionEventApplier {
	return &ReflectionEventApplier{}
}

// ApplyEvent aplica un evento usando reflexión
func (r *ReflectionEventApplier) ApplyEvent(aggregate interface{}, event *Event) error {
	aggregateValue := reflect.ValueOf(aggregate)
	if aggregateValue.Kind() == reflect.Ptr {
		aggregateValue = aggregateValue.Elem()
	}
	
	// Buscar método Apply{EventType}
	methodName := "Apply" + event.EventType
	method := aggregateValue.Addr().MethodByName(methodName)
	
	if !method.IsValid() {
		return fmt.Errorf("method %s not found on aggregate %T", methodName, aggregate)
	}
	
	// Verificar signatura del método
	methodType := method.Type()
	if methodType.NumIn() != 1 || methodType.NumOut() != 1 {
		return fmt.Errorf("method %s must have signature (event *Event) error", methodName)
	}
	
	// Llamar al método
	results := method.Call([]reflect.Value{reflect.ValueOf(event)})
	
	// Verificar si retornó error
	if !results[0].IsNil() {
		return results[0].Interface().(error)
	}
	
	return nil
}
