package eventstore

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// eventSubscriber representa un suscriptor con su filtro
type eventSubscriber struct {
	channel chan *Event
	filter  EventFilter
}

// MemoryEventStore implementa EventStore usando almacenamiento en memoria
// Útil para testing y desarrollo, no recomendado para producción
type MemoryEventStore struct {
	// Mutex para proteger acceso concurrente
	mu sync.RWMutex

	// Slice que almacena todos los eventos ordenados por sequence_number
	events []*Event

	// Mapa para acceso rápido por aggregate_id
	eventsByAggregate map[string][]*Event

	// Mapa para almacenar snapshots
	snapshots map[string]*Snapshot

	// Contador global de secuencia
	sequenceCounter int64

	// Mapa para rastrear versiones de agregados
	aggregateVersions map[string]int64

	// Canales para suscripciones en tiempo real
	subscribers []chan *Event

	// Suscriptores con filtros
	eventSubscribers []*eventSubscriber

	// Flag para indicar si el store está cerrado
	closed bool
}

// NewMemoryEventStore crea una nueva instancia del event store en memoria
func NewMemoryEventStore() *MemoryEventStore {
	return &MemoryEventStore{
		events:            make([]*Event, 0),
		eventsByAggregate: make(map[string][]*Event),
		snapshots:         make(map[string]*Snapshot),
		sequenceCounter:   0,
		aggregateVersions: make(map[string]int64),
		subscribers:       make([]chan *Event, 0),
		eventSubscribers:  make([]*eventSubscriber, 0),
		closed:            false,
	}
}

// AppendEvent guarda un nuevo evento en el store
func (m *MemoryEventStore) AppendEvent(ctx context.Context, event *Event) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return fmt.Errorf("event store is closed")
	}

	// Validar que el evento tenga datos requeridos
	if event.AggregateID == "" || event.AggregateType == "" || event.EventType == "" {
		return fmt.Errorf("event must have aggregate_id, aggregate_type, and event_type")
	}

	// Obtener la versión actual del agregado
	currentVersion := m.aggregateVersions[event.AggregateID]

	// Verificar concurrencia optimista si se especifica una versión esperada
	if event.AggregateVersion > 0 && event.AggregateVersion != currentVersion+1 {
		return fmt.Errorf("concurrency conflict: expected version %d, got %d",
			currentVersion+1, event.AggregateVersion)
	}

	// Asignar número de secuencia y versión
	m.sequenceCounter++
	event.SequenceNumber = m.sequenceCounter
	event.AggregateVersion = currentVersion + 1

	// Actualizar timestamp si no está establecido
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}

	// Agregar evento a la lista principal
	m.events = append(m.events, event)

	// Agregar evento al índice por agregado
	m.eventsByAggregate[event.AggregateID] = append(
		m.eventsByAggregate[event.AggregateID], event)

	// Actualizar versión del agregado
	m.aggregateVersions[event.AggregateID] = event.AggregateVersion

	// Notificar a suscriptores
	m.notifySubscribers(event)

	return nil
}

// AppendEvents guarda múltiples eventos de forma atómica
func (m *MemoryEventStore) AppendEvents(ctx context.Context, events []*Event) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	if len(events) == 0 {
		return nil
	}

	// Verificar que todos los eventos sean del mismo agregado
	aggregateID := events[0].AggregateID
	for _, event := range events {
		if event.AggregateID != aggregateID {
			return fmt.Errorf("all events must belong to the same aggregate")
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return fmt.Errorf("event store is closed")
	}

	// Obtener versión actual del agregado
	currentVersion := m.aggregateVersions[aggregateID]

	// Validar y asignar versiones secuenciales
	for i, event := range events {
		expectedVersion := currentVersion + int64(i) + 1

		if event.AggregateVersion > 0 && event.AggregateVersion != expectedVersion {
			return fmt.Errorf("concurrency conflict at event %d: expected version %d, got %d",
				i, expectedVersion, event.AggregateVersion)
		}

		// Asignar número de secuencia y versión
		m.sequenceCounter++
		event.SequenceNumber = m.sequenceCounter
		event.AggregateVersion = expectedVersion

		if event.CreatedAt.IsZero() {
			event.CreatedAt = time.Now().UTC()
		}
	}

	// Agregar todos los eventos
	for _, event := range events {
		m.events = append(m.events, event)
		m.eventsByAggregate[event.AggregateID] = append(
			m.eventsByAggregate[event.AggregateID], event)
		m.notifySubscribers(event)
	}

	// Actualizar versión final del agregado
	m.aggregateVersions[aggregateID] = events[len(events)-1].AggregateVersion

	return nil
}

// GetEvents recupera eventos aplicando filtros
func (m *MemoryEventStore) GetEvents(ctx context.Context, filter EventFilter) ([]*Event, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*Event

	for _, event := range m.events {
		if m.matchesFilter(event, filter) {
			result = append(result, event)
		}
	}

	// Aplicar límite y offset
	if filter.Offset != nil && *filter.Offset > 0 {
		if *filter.Offset >= len(result) {
			return []*Event{}, nil
		}
		result = result[*filter.Offset:]
	}

	if filter.Limit != nil && *filter.Limit > 0 && len(result) > *filter.Limit {
		result = result[:*filter.Limit]
	}

	return result, nil
}

// GetEventsByAggregate recupera todos los eventos de un agregado específico
func (m *MemoryEventStore) GetEventsByAggregate(ctx context.Context, aggregateID string, fromVersion int64) ([]*Event, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	events, exists := m.eventsByAggregate[aggregateID]
	if !exists {
		return []*Event{}, nil
	}

	var result []*Event
	for _, event := range events {
		if event.AggregateVersion >= fromVersion {
			result = append(result, event)
		}
	}

	return result, nil
}

// GetEventsByAggregateType recupera eventos de un tipo de agregado específico
func (m *MemoryEventStore) GetEventsByAggregateType(ctx context.Context, aggregateType string, filter EventFilter) ([]*Event, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Agregar filtro por tipo de agregado
	filter.AggregateType = &aggregateType

	return m.GetEvents(ctx, filter)
}

// GetLastSequenceNumber retorna el último número de secuencia usado
func (m *MemoryEventStore) GetLastSequenceNumber(ctx context.Context) (int64, error) {
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.sequenceCounter, nil
}

// GetAggregateVersion retorna la versión actual de un agregado
func (m *MemoryEventStore) GetAggregateVersion(ctx context.Context, aggregateID string) (int64, error) {
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	version, exists := m.aggregateVersions[aggregateID]
	if !exists {
		return 0, nil
	}

	return version, nil
}

// CreateSnapshot crea un snapshot del estado actual de un agregado
func (m *MemoryEventStore) CreateSnapshot(ctx context.Context, aggregateID string, aggregateType string, version int64, data json.RawMessage) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	snapshot := &Snapshot{
		AggregateID:      aggregateID,
		AggregateType:    aggregateType,
		AggregateVersion: version,
		Data:             data,
		CreatedAt:        time.Now().UTC(),
	}

	m.snapshots[aggregateID] = snapshot

	return nil
}

// GetSnapshot recupera el snapshot más reciente de un agregado
func (m *MemoryEventStore) GetSnapshot(ctx context.Context, aggregateID string) (*Snapshot, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	snapshot, exists := m.snapshots[aggregateID]
	if !exists {
		return nil, nil
	}

	return snapshot, nil
}

// Subscribe permite suscribirse a nuevos eventos en tiempo real
func (m *MemoryEventStore) Subscribe(ctx context.Context, filter EventFilter) (<-chan *Event, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil, fmt.Errorf("event store is closed")
	}

	// Crear canal para el suscriptor
	eventChan := make(chan *Event, 100) // Buffer para evitar bloqueos

	// Crear estructura de suscriptor con filtro
	subscriber := &eventSubscriber{
		channel: eventChan,
		filter:  filter,
	}

	// Agregar a la lista de suscriptores
	m.eventSubscribers = append(m.eventSubscribers, subscriber)

	// Solo enviar eventos existentes si no hay filtro de secuencia
	// Si hay FromSequenceNumber, significa que ya se procesaron eventos hasta ese punto
	if filter.FromSequenceNumber == nil {
		go func() {
			m.mu.RLock()
			existingEvents := make([]*Event, len(m.events))
			copy(existingEvents, m.events)
			m.mu.RUnlock()

			for _, event := range existingEvents {
				if m.matchesFilter(event, filter) {
					select {
					case eventChan <- event:
					case <-ctx.Done():
						return
					default:
						// Canal lleno, saltar evento
					}
				}
			}
		}()
	}

	// Goroutine para manejar la limpieza cuando se cancele el contexto
	go func() {
		<-ctx.Done()
		m.mu.Lock()
		defer m.mu.Unlock()

		// Remover suscriptor y cerrar canal
		for i, sub := range m.eventSubscribers {
			if sub.channel == eventChan {
				m.eventSubscribers = append(m.eventSubscribers[:i], m.eventSubscribers[i+1:]...)
				close(eventChan)
				break
			}
		}
	}()

	return eventChan, nil
}

// Close cierra las conexiones y libera recursos
func (m *MemoryEventStore) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil
	}

	m.closed = true

	// Cerrar todos los canales de suscriptores
	for _, sub := range m.subscribers {
		close(sub)
	}
	m.subscribers = nil

	return nil
}

// matchesFilter verifica si un evento coincide con los criterios del filtro
func (m *MemoryEventStore) matchesFilter(event *Event, filter EventFilter) bool {
	if filter.AggregateID != nil && event.AggregateID != *filter.AggregateID {
		return false
	}

	if filter.AggregateType != nil && event.AggregateType != *filter.AggregateType {
		return false
	}

	if len(filter.EventTypes) > 0 {
		found := false
		for _, eventType := range filter.EventTypes {
			if event.EventType == eventType {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	if filter.FromAggregateVersion != nil && event.AggregateVersion < *filter.FromAggregateVersion {
		return false
	}

	if filter.ToAggregateVersion != nil && event.AggregateVersion > *filter.ToAggregateVersion {
		return false
	}

	if filter.FromSequenceNumber != nil && event.SequenceNumber <= *filter.FromSequenceNumber {
		return false
	}

	if filter.ToSequenceNumber != nil && event.SequenceNumber > *filter.ToSequenceNumber {
		return false
	}

	if filter.FromTimestamp != nil && event.CreatedAt.Before(*filter.FromTimestamp) {
		return false
	}

	if filter.ToTimestamp != nil && event.CreatedAt.After(*filter.ToTimestamp) {
		return false
	}

	return true
}

// notifySubscribers envía el evento a todos los suscriptores activos
func (m *MemoryEventStore) notifySubscribers(event *Event) {
	// Notificar suscriptores legacy (sin filtro)
	for i := len(m.subscribers) - 1; i >= 0; i-- {
		select {
		case m.subscribers[i] <- event:
			// Evento enviado exitosamente
		default:
			// Canal lleno o cerrado, remover suscriptor
			close(m.subscribers[i])
			m.subscribers = append(m.subscribers[:i], m.subscribers[i+1:]...)
		}
	}

	// Notificar suscriptores con filtros
	for i := len(m.eventSubscribers) - 1; i >= 0; i-- {
		subscriber := m.eventSubscribers[i]
		if m.matchesFilter(event, subscriber.filter) {
			select {
			case subscriber.channel <- event:
				// Evento enviado exitosamente
			default:
				// Canal lleno o cerrado, remover suscriptor
				close(subscriber.channel)
				m.eventSubscribers = append(m.eventSubscribers[:i], m.eventSubscribers[i+1:]...)
			}
		}
	}
}
