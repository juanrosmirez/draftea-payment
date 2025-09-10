package eventstore

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Event representa un evento almacenado en el Event Store
type Event struct {
	// ID único del evento
	ID string `json:"id" db:"id"`
	
	// ID del agregado al que pertenece el evento
	AggregateID string `json:"aggregate_id" db:"aggregate_id"`
	
	// Tipo del agregado (payment, wallet, etc.)
	AggregateType string `json:"aggregate_type" db:"aggregate_type"`
	
	// Tipo del evento (PaymentInitiated, WalletDeducted, etc.)
	EventType string `json:"event_type" db:"event_type"`
	
	// Versión del agregado después de aplicar este evento
	AggregateVersion int64 `json:"aggregate_version" db:"aggregate_version"`
	
	// Número de secuencia global del evento
	SequenceNumber int64 `json:"sequence_number" db:"sequence_number"`
	
	// Datos del evento en formato JSON
	EventData json.RawMessage `json:"event_data" db:"event_data"`
	
	// Metadatos adicionales del evento
	Metadata json.RawMessage `json:"metadata" db:"metadata"`
	
	// Timestamp de cuando se creó el evento
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	
	// ID del usuario que causó el evento (para auditoría)
	CausedBy string `json:"caused_by" db:"caused_by"`
	
	// ID de correlación para rastrear flujos de eventos relacionados
	CorrelationID string `json:"correlation_id" db:"correlation_id"`
}

// EventFilter define criterios para filtrar eventos
type EventFilter struct {
	// Filtrar por ID de agregado específico
	AggregateID *string
	
	// Filtrar por tipo de agregado
	AggregateType *string
	
	// Filtrar por tipos de eventos específicos
	EventTypes []string
	
	// Filtrar eventos desde una versión específica del agregado
	FromAggregateVersion *int64
	
	// Filtrar eventos hasta una versión específica del agregado
	ToAggregateVersion *int64
	
	// Filtrar eventos desde un número de secuencia específico
	FromSequenceNumber *int64
	
	// Filtrar eventos hasta un número de secuencia específico
	ToSequenceNumber *int64
	
	// Filtrar eventos desde una fecha específica
	FromTimestamp *time.Time
	
	// Filtrar eventos hasta una fecha específica
	ToTimestamp *time.Time
	
	// Límite de eventos a retornar
	Limit *int
	
	// Offset para paginación
	Offset *int
}

// EventStore define la interfaz para el almacén de eventos
type EventStore interface {
	// AppendEvent guarda un nuevo evento en el store
	// Retorna error si hay conflicto de concurrencia o falla de persistencia
	AppendEvent(ctx context.Context, event *Event) error
	
	// AppendEvents guarda múltiples eventos de forma atómica
	// Todos los eventos deben ser del mismo agregado para garantizar atomicidad
	AppendEvents(ctx context.Context, events []*Event) error
	
	// GetEvents recupera eventos aplicando los filtros especificados
	// Los eventos se retornan ordenados por sequence_number ascendente
	GetEvents(ctx context.Context, filter EventFilter) ([]*Event, error)
	
	// GetEventsByAggregate recupera todos los eventos de un agregado específico
	// Útil para reconstruir el estado completo de un agregado
	GetEventsByAggregate(ctx context.Context, aggregateID string, fromVersion int64) ([]*Event, error)
	
	// GetEventsByAggregateType recupera eventos de un tipo de agregado específico
	// Útil para proyecciones y vistas materializadas
	GetEventsByAggregateType(ctx context.Context, aggregateType string, filter EventFilter) ([]*Event, error)
	
	// GetLastSequenceNumber retorna el último número de secuencia usado
	// Útil para implementar suscripciones a eventos en tiempo real
	GetLastSequenceNumber(ctx context.Context) (int64, error)
	
	// GetAggregateVersion retorna la versión actual de un agregado
	// Útil para detectar conflictos de concurrencia
	GetAggregateVersion(ctx context.Context, aggregateID string) (int64, error)
	
	// CreateSnapshot crea un snapshot del estado actual de un agregado
	// Optimización para agregados con muchos eventos
	CreateSnapshot(ctx context.Context, aggregateID string, aggregateType string, version int64, data json.RawMessage) error
	
	// GetSnapshot recupera el snapshot más reciente de un agregado
	GetSnapshot(ctx context.Context, aggregateID string) (*Snapshot, error)
	
	// Subscribe permite suscribirse a nuevos eventos en tiempo real
	// Retorna un canal que recibe eventos conforme se van agregando
	Subscribe(ctx context.Context, filter EventFilter) (<-chan *Event, error)
	
	// Close cierra las conexiones y libera recursos
	Close() error
}

// Snapshot representa un snapshot del estado de un agregado
type Snapshot struct {
	AggregateID      string          `json:"aggregate_id" db:"aggregate_id"`
	AggregateType    string          `json:"aggregate_type" db:"aggregate_type"`
	AggregateVersion int64           `json:"aggregate_version" db:"aggregate_version"`
	Data             json.RawMessage `json:"data" db:"data"`
	CreatedAt        time.Time       `json:"created_at" db:"created_at"`
}

// NewEvent crea un nuevo evento con metadatos básicos
func NewEvent(aggregateID, aggregateType, eventType string, eventData interface{}, causedBy, correlationID string) (*Event, error) {
	eventDataBytes, err := json.Marshal(eventData)
	if err != nil {
		return nil, err
	}
	
	metadata := map[string]interface{}{
		"source":    "payment-system",
		"version":   "1.0",
		"timestamp": time.Now().UTC(),
	}
	
	metadataBytes, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}
	
	return &Event{
		ID:            uuid.New().String(),
		AggregateID:   aggregateID,
		AggregateType: aggregateType,
		EventType:     eventType,
		EventData:     eventDataBytes,
		Metadata:      metadataBytes,
		CreatedAt:     time.Now().UTC(),
		CausedBy:      causedBy,
		CorrelationID: correlationID,
	}, nil
}

// UnmarshalEventData deserializa los datos del evento a la estructura especificada
func (e *Event) UnmarshalEventData(target interface{}) error {
	return json.Unmarshal(e.EventData, target)
}

// UnmarshalMetadata deserializa los metadatos del evento
func (e *Event) UnmarshalMetadata(target interface{}) error {
	return json.Unmarshal(e.Metadata, target)
}
