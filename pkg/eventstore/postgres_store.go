package eventstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/lib/pq"
	_ "github.com/lib/pq"
)

// PostgresEventStore implementa EventStore usando PostgreSQL
// Proporciona persistencia durable y manejo robusto de concurrencia
type PostgresEventStore struct {
	db *sql.DB
	dsn string
	
	// Configuración para optimistic locking
	enableOptimisticLocking bool
	
	// Configuración para notificaciones en tiempo real
	enableNotifications bool
	notificationChannel string
}

// PostgresConfig configuración para el event store de PostgreSQL
type PostgresConfig struct {
	// Cadena de conexión a PostgreSQL
	DSN string
	
	// Habilitar optimistic locking para prevenir conflictos de concurrencia
	EnableOptimisticLocking bool
	
	// Habilitar notificaciones LISTEN/NOTIFY para suscripciones en tiempo real
	EnableNotifications bool
	
	// Canal de notificación para eventos nuevos
	NotificationChannel string
	
	// Configuración de pool de conexiones
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

// NewPostgresEventStore crea una nueva instancia del event store con PostgreSQL
func NewPostgresEventStore(config PostgresConfig) (*PostgresEventStore, error) {
	db, err := sql.Open("postgres", config.DSN)
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection: %w", err)
	}
	
	// Configurar pool de conexiones
	if config.MaxOpenConns > 0 {
		db.SetMaxOpenConns(config.MaxOpenConns)
	}
	if config.MaxIdleConns > 0 {
		db.SetMaxIdleConns(config.MaxIdleConns)
	}
	if config.ConnMaxLifetime > 0 {
		db.SetConnMaxLifetime(config.ConnMaxLifetime)
	}
	
	// Verificar conexión
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}
	
	store := &PostgresEventStore{
		db:                      db,
		dsn:                     config.DSN,
		enableOptimisticLocking: config.EnableOptimisticLocking,
		enableNotifications:     config.EnableNotifications,
		notificationChannel:     config.NotificationChannel,
	}
	
	// Crear tablas si no existen
	if err := store.createTables(); err != nil {
		return nil, fmt.Errorf("failed to create tables: %w", err)
	}
	
	return store, nil
}

// createTables crea las tablas necesarias para el event store
func (p *PostgresEventStore) createTables() error {
	// Tabla principal de eventos
	eventsTableSQL := `
	CREATE TABLE IF NOT EXISTS events (
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
		
		-- Constraint para garantizar versiones secuenciales por agregado
		CONSTRAINT unique_aggregate_version UNIQUE (aggregate_id, aggregate_version)
	);
	
	-- Índices para optimizar consultas
	CREATE INDEX IF NOT EXISTS idx_events_aggregate_id ON events (aggregate_id);
	CREATE INDEX IF NOT EXISTS idx_events_aggregate_type ON events (aggregate_type);
	CREATE INDEX IF NOT EXISTS idx_events_event_type ON events (event_type);
	CREATE INDEX IF NOT EXISTS idx_events_sequence_number ON events (sequence_number);
	CREATE INDEX IF NOT EXISTS idx_events_created_at ON events (created_at);
	CREATE INDEX IF NOT EXISTS idx_events_correlation_id ON events (correlation_id);
	
	-- Índice compuesto para consultas por agregado y versión
	CREATE INDEX IF NOT EXISTS idx_events_aggregate_version ON events (aggregate_id, aggregate_version);
	`
	
	// Tabla de snapshots
	snapshotsTableSQL := `
	CREATE TABLE IF NOT EXISTS snapshots (
		aggregate_id VARCHAR(36) PRIMARY KEY,
		aggregate_type VARCHAR(100) NOT NULL,
		aggregate_version BIGINT NOT NULL,
		data JSONB NOT NULL,
		created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
	);
	
	CREATE INDEX IF NOT EXISTS idx_snapshots_aggregate_type ON snapshots (aggregate_type);
	CREATE INDEX IF NOT EXISTS idx_snapshots_created_at ON snapshots (created_at);
	`
	
	// Función para notificaciones automáticas (si están habilitadas)
	var notificationFunctionSQL string
	if p.enableNotifications {
		notificationFunctionSQL = fmt.Sprintf(`
		CREATE OR REPLACE FUNCTION notify_event_inserted()
		RETURNS TRIGGER AS $$
		BEGIN
			PERFORM pg_notify('%s', json_build_object(
				'event_id', NEW.id,
				'aggregate_id', NEW.aggregate_id,
				'event_type', NEW.event_type,
				'sequence_number', NEW.sequence_number
			)::text);
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;
		
		DROP TRIGGER IF EXISTS trigger_notify_event_inserted ON events;
		CREATE TRIGGER trigger_notify_event_inserted
			AFTER INSERT ON events
			FOR EACH ROW
			EXECUTE FUNCTION notify_event_inserted();
		`, p.notificationChannel)
	}
	
	// Ejecutar todas las sentencias DDL
	statements := []string{eventsTableSQL, snapshotsTableSQL}
	if notificationFunctionSQL != "" {
		statements = append(statements, notificationFunctionSQL)
	}
	
	for _, stmt := range statements {
		if _, err := p.db.Exec(stmt); err != nil {
			return fmt.Errorf("failed to execute DDL statement: %w", err)
		}
	}
	
	return nil
}

// AppendEvent guarda un nuevo evento en el store
func (p *PostgresEventStore) AppendEvent(ctx context.Context, event *Event) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	
	// Validaciones básicas
	if event.AggregateID == "" || event.AggregateType == "" || event.EventType == "" {
		return fmt.Errorf("event must have aggregate_id, aggregate_type, and event_type")
	}
	
	// Iniciar transacción para garantizar atomicidad
	tx, err := p.db.BeginTx(ctx, &sql.TxOptions{
		Isolation: sql.LevelSerializable, // Máximo nivel de aislamiento
	})
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()
	
	// Obtener la versión actual del agregado con lock
	var currentVersion int64
	err = tx.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(aggregate_version), 0) 
		FROM events 
		WHERE aggregate_id = $1 
		FOR UPDATE
	`, event.AggregateID).Scan(&currentVersion)
	
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to get current aggregate version: %w", err)
	}
	
	// Verificar concurrencia optimista si está habilitada
	if p.enableOptimisticLocking && event.AggregateVersion > 0 {
		expectedVersion := currentVersion + 1
		if event.AggregateVersion != expectedVersion {
			return fmt.Errorf("concurrency conflict: expected version %d, got %d", 
				expectedVersion, event.AggregateVersion)
		}
	}
	
	// Asignar la siguiente versión
	event.AggregateVersion = currentVersion + 1
	
	// Establecer timestamp si no está definido
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	
	// Insertar el evento
	_, err = tx.ExecContext(ctx, `
		INSERT INTO events (
			id, aggregate_id, aggregate_type, event_type, 
			aggregate_version, event_data, metadata, 
			created_at, caused_by, correlation_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, event.ID, event.AggregateID, event.AggregateType, event.EventType,
		event.AggregateVersion, event.EventData, event.Metadata,
		event.CreatedAt, event.CausedBy, event.CorrelationID)
	
	if err != nil {
		// Verificar si es un error de constraint de versión duplicada
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Constraint == "unique_aggregate_version" {
			return fmt.Errorf("concurrency conflict: version %d already exists for aggregate %s", 
				event.AggregateVersion, event.AggregateID)
		}
		return fmt.Errorf("failed to insert event: %w", err)
	}
	
	// Obtener el sequence_number generado
	err = tx.QueryRowContext(ctx, `
		SELECT sequence_number FROM events WHERE id = $1
	`, event.ID).Scan(&event.SequenceNumber)
	
	if err != nil {
		return fmt.Errorf("failed to get sequence number: %w", err)
	}
	
	// Commit de la transacción
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	
	return nil
}

// AppendEvents guarda múltiples eventos de forma atómica
func (p *PostgresEventStore) AppendEvents(ctx context.Context, events []*Event) error {
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
	
	// Iniciar transacción
	tx, err := p.db.BeginTx(ctx, &sql.TxOptions{
		Isolation: sql.LevelSerializable,
	})
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()
	
	// Obtener versión actual con lock
	var currentVersion int64
	err = tx.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(aggregate_version), 0) 
		FROM events 
		WHERE aggregate_id = $1 
		FOR UPDATE
	`, aggregateID).Scan(&currentVersion)
	
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to get current aggregate version: %w", err)
	}
	
	// Preparar statement para inserción batch
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO events (
			id, aggregate_id, aggregate_type, event_type, 
			aggregate_version, event_data, metadata, 
			created_at, caused_by, correlation_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()
	
	// Insertar eventos secuencialmente
	for i, event := range events {
		event.AggregateVersion = currentVersion + int64(i) + 1
		
		if event.CreatedAt.IsZero() {
			event.CreatedAt = time.Now().UTC()
		}
		
		_, err = stmt.ExecContext(ctx,
			event.ID, event.AggregateID, event.AggregateType, event.EventType,
			event.AggregateVersion, event.EventData, event.Metadata,
			event.CreatedAt, event.CausedBy, event.CorrelationID)
		
		if err != nil {
			return fmt.Errorf("failed to insert event %d: %w", i, err)
		}
	}
	
	// Obtener sequence numbers generados
	rows, err := tx.QueryContext(ctx, `
		SELECT id, sequence_number 
		FROM events 
		WHERE aggregate_id = $1 AND aggregate_version > $2
		ORDER BY aggregate_version
	`, aggregateID, currentVersion)
	
	if err != nil {
		return fmt.Errorf("failed to get sequence numbers: %w", err)
	}
	defer rows.Close()
	
	// Mapear sequence numbers a eventos
	eventMap := make(map[string]*Event)
	for _, event := range events {
		eventMap[event.ID] = event
	}
	
	for rows.Next() {
		var eventID string
		var sequenceNumber int64
		
		if err := rows.Scan(&eventID, &sequenceNumber); err != nil {
			return fmt.Errorf("failed to scan sequence number: %w", err)
		}
		
		if event, exists := eventMap[eventID]; exists {
			event.SequenceNumber = sequenceNumber
		}
	}
	
	// Commit de la transacción
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	
	return nil
}

// GetEvents recupera eventos aplicando filtros
func (p *PostgresEventStore) GetEvents(ctx context.Context, filter EventFilter) ([]*Event, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	
	// Construir query dinámicamente basado en filtros
	query := "SELECT id, aggregate_id, aggregate_type, event_type, aggregate_version, sequence_number, event_data, metadata, created_at, caused_by, correlation_id FROM events WHERE 1=1"
	args := make([]interface{}, 0)
	argIndex := 1
	
	// Aplicar filtros
	if filter.AggregateID != nil {
		query += fmt.Sprintf(" AND aggregate_id = $%d", argIndex)
		args = append(args, *filter.AggregateID)
		argIndex++
	}
	
	if filter.AggregateType != nil {
		query += fmt.Sprintf(" AND aggregate_type = $%d", argIndex)
		args = append(args, *filter.AggregateType)
		argIndex++
	}
	
	if len(filter.EventTypes) > 0 {
		query += fmt.Sprintf(" AND event_type = ANY($%d)", argIndex)
		args = append(args, pq.Array(filter.EventTypes))
		argIndex++
	}
	
	if filter.FromAggregateVersion != nil {
		query += fmt.Sprintf(" AND aggregate_version >= $%d", argIndex)
		args = append(args, *filter.FromAggregateVersion)
		argIndex++
	}
	
	if filter.ToAggregateVersion != nil {
		query += fmt.Sprintf(" AND aggregate_version <= $%d", argIndex)
		args = append(args, *filter.ToAggregateVersion)
		argIndex++
	}
	
	if filter.FromSequenceNumber != nil {
		query += fmt.Sprintf(" AND sequence_number >= $%d", argIndex)
		args = append(args, *filter.FromSequenceNumber)
		argIndex++
	}
	
	if filter.ToSequenceNumber != nil {
		query += fmt.Sprintf(" AND sequence_number <= $%d", argIndex)
		args = append(args, *filter.ToSequenceNumber)
		argIndex++
	}
	
	if filter.FromTimestamp != nil {
		query += fmt.Sprintf(" AND created_at >= $%d", argIndex)
		args = append(args, *filter.FromTimestamp)
		argIndex++
	}
	
	if filter.ToTimestamp != nil {
		query += fmt.Sprintf(" AND created_at <= $%d", argIndex)
		args = append(args, *filter.ToTimestamp)
		argIndex++
	}
	
	// Ordenar por sequence_number
	query += " ORDER BY sequence_number ASC"
	
	// Aplicar límite y offset
	if filter.Limit != nil {
		query += fmt.Sprintf(" LIMIT $%d", argIndex)
		args = append(args, *filter.Limit)
		argIndex++
	}
	
	if filter.Offset != nil {
		query += fmt.Sprintf(" OFFSET $%d", argIndex)
		args = append(args, *filter.Offset)
		argIndex++
	}
	
	// Ejecutar query
	rows, err := p.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query events: %w", err)
	}
	defer rows.Close()
	
	// Escanear resultados
	var events []*Event
	for rows.Next() {
		event := &Event{}
		err := rows.Scan(
			&event.ID, &event.AggregateID, &event.AggregateType, &event.EventType,
			&event.AggregateVersion, &event.SequenceNumber, &event.EventData,
			&event.Metadata, &event.CreatedAt, &event.CausedBy, &event.CorrelationID,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan event: %w", err)
		}
		events = append(events, event)
	}
	
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}
	
	return events, nil
}

// GetEventsByAggregate recupera todos los eventos de un agregado específico
func (p *PostgresEventStore) GetEventsByAggregate(ctx context.Context, aggregateID string, fromVersion int64) ([]*Event, error) {
	filter := EventFilter{
		AggregateID:          &aggregateID,
		FromAggregateVersion: &fromVersion,
	}
	return p.GetEvents(ctx, filter)
}

// GetEventsByAggregateType recupera eventos de un tipo de agregado específico
func (p *PostgresEventStore) GetEventsByAggregateType(ctx context.Context, aggregateType string, filter EventFilter) ([]*Event, error) {
	filter.AggregateType = &aggregateType
	return p.GetEvents(ctx, filter)
}

// GetLastSequenceNumber retorna el último número de secuencia usado
func (p *PostgresEventStore) GetLastSequenceNumber(ctx context.Context) (int64, error) {
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}
	
	var lastSequence int64
	err := p.db.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(sequence_number), 0) FROM events
	`).Scan(&lastSequence)
	
	if err != nil {
		return 0, fmt.Errorf("failed to get last sequence number: %w", err)
	}
	
	return lastSequence, nil
}

// GetAggregateVersion retorna la versión actual de un agregado
func (p *PostgresEventStore) GetAggregateVersion(ctx context.Context, aggregateID string) (int64, error) {
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}
	
	var version int64
	err := p.db.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(aggregate_version), 0) 
		FROM events 
		WHERE aggregate_id = $1
	`, aggregateID).Scan(&version)
	
	if err != nil {
		return 0, fmt.Errorf("failed to get aggregate version: %w", err)
	}
	
	return version, nil
}

// CreateSnapshot crea un snapshot del estado actual de un agregado
func (p *PostgresEventStore) CreateSnapshot(ctx context.Context, aggregateID string, aggregateType string, version int64, data json.RawMessage) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	
	_, err := p.db.ExecContext(ctx, `
		INSERT INTO snapshots (aggregate_id, aggregate_type, aggregate_version, data)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (aggregate_id) 
		DO UPDATE SET 
			aggregate_type = EXCLUDED.aggregate_type,
			aggregate_version = EXCLUDED.aggregate_version,
			data = EXCLUDED.data,
			created_at = NOW()
	`, aggregateID, aggregateType, version, data)
	
	if err != nil {
		return fmt.Errorf("failed to create snapshot: %w", err)
	}
	
	return nil
}

// GetSnapshot recupera el snapshot más reciente de un agregado
func (p *PostgresEventStore) GetSnapshot(ctx context.Context, aggregateID string) (*Snapshot, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	
	snapshot := &Snapshot{}
	err := p.db.QueryRowContext(ctx, `
		SELECT aggregate_id, aggregate_type, aggregate_version, data, created_at
		FROM snapshots 
		WHERE aggregate_id = $1
	`, aggregateID).Scan(
		&snapshot.AggregateID, &snapshot.AggregateType,
		&snapshot.AggregateVersion, &snapshot.Data, &snapshot.CreatedAt,
	)
	
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get snapshot: %w", err)
	}
	
	return snapshot, nil
}

// Subscribe permite suscribirse a nuevos eventos en tiempo real usando LISTEN/NOTIFY
func (p *PostgresEventStore) Subscribe(ctx context.Context, filter EventFilter) (<-chan *Event, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	
	if !p.enableNotifications {
		return nil, fmt.Errorf("notifications are not enabled")
	}
	
	// Crear conexión dedicada para LISTEN usando el DSN almacenado
	listener := pq.NewListener(p.dsn, 10*time.Second, time.Minute, nil)
	
	// Escuchar el canal de notificaciones
	err := listener.Listen(p.notificationChannel)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on channel: %w", err)
	}
	
	eventChan := make(chan *Event, 100)
	
	// Goroutine para manejar notificaciones
	go func() {
		defer close(eventChan)
		defer listener.Close()
		
		for {
			select {
			case <-ctx.Done():
				return
			case notification := <-listener.Notify:
				if notification != nil {
					// Parsear la notificación y obtener el evento completo
					var notificationData struct {
						EventID        string `json:"event_id"`
						AggregateID    string `json:"aggregate_id"`
						EventType      string `json:"event_type"`
						SequenceNumber int64  `json:"sequence_number"`
					}
					
					if err := json.Unmarshal([]byte(notification.Extra), &notificationData); err != nil {
						continue
					}
					
					// Obtener el evento completo de la base de datos
					events, err := p.GetEvents(ctx, EventFilter{
						AggregateID: &notificationData.AggregateID,
						FromSequenceNumber: &notificationData.SequenceNumber,
						ToSequenceNumber: &notificationData.SequenceNumber,
					})
					
					if err != nil || len(events) == 0 {
						continue
					}
					
					event := events[0]
					
					// Verificar si el evento coincide con el filtro
					if p.matchesFilter(event, filter) {
						select {
						case eventChan <- event:
						case <-ctx.Done():
							return
						}
					}
				}
			}
		}
	}()
	
	return eventChan, nil
}

// Close cierra las conexiones y libera recursos
func (p *PostgresEventStore) Close() error {
	return p.db.Close()
}

// matchesFilter verifica si un evento coincide con los criterios del filtro
// (Reutiliza la lógica de MemoryEventStore)
func (p *PostgresEventStore) matchesFilter(event *Event, filter EventFilter) bool {
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
	
	if filter.FromSequenceNumber != nil && event.SequenceNumber < *filter.FromSequenceNumber {
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
