package readmodels

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"payment-system/pkg/eventstore"
)

// ReadModelStore define la interfaz para persistir read models
type ReadModelStore interface {
	// Set almacena un valor en el read model store
	Set(ctx context.Context, key string, value interface{}) error
	
	// Get recupera un valor del read model store
	Get(ctx context.Context, key string, target interface{}) error
	
	// Delete elimina un valor del read model store
	Delete(ctx context.Context, key string) error
	
	// GetAll recupera todos los valores que coinciden con un patrón
	GetAll(ctx context.Context, pattern string) (map[string]interface{}, error)
	
	// Close cierra las conexiones del store
	Close() error
}

// MemoryReadModelStore implementa ReadModelStore en memoria
type MemoryReadModelStore struct {
	mu   sync.RWMutex
	data map[string]interface{}
}

// NewMemoryReadModelStore crea un nuevo store en memoria
func NewMemoryReadModelStore() *MemoryReadModelStore {
	return &MemoryReadModelStore{
		data: make(map[string]interface{}),
	}
}

func (m *MemoryReadModelStore) Set(ctx context.Context, key string, value interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = value
	return nil
}

func (m *MemoryReadModelStore) Get(ctx context.Context, key string, target interface{}) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	value, exists := m.data[key]
	if !exists {
		return fmt.Errorf("key not found: %s", key)
	}
	
	// Serializar y deserializar para copiar el valor
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	
	return json.Unmarshal(data, target)
}

func (m *MemoryReadModelStore) Delete(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}

func (m *MemoryReadModelStore) GetAll(ctx context.Context, pattern string) (map[string]interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	result := make(map[string]interface{})
	for key, value := range m.data {
		// Implementación simple de pattern matching
		if pattern == "*" || key == pattern {
			result[key] = value
		} else if strings.HasSuffix(pattern, "*") {
			// Patrón con wildcard al final (ej: "balance:*")
			prefix := strings.TrimSuffix(pattern, "*")
			if strings.HasPrefix(key, prefix) {
				result[key] = value
			}
		}
	}
	
	return result, nil
}

func (m *MemoryReadModelStore) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data = nil
	return nil
}

// BalanceReadModel representa el saldo actual de una billetera
type BalanceReadModel struct {
	UserID           string                 `json:"user_id"`
	Currency         string                 `json:"currency"`
	Balance          float64                `json:"balance"`
	LastUpdated      time.Time              `json:"last_updated"`
	LastEventVersion int64                  `json:"last_event_version"`
	TransactionCount int64                  `json:"transaction_count"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
}

// WalletSummaryReadModel representa un resumen de la billetera
type WalletSummaryReadModel struct {
	UserID              string    `json:"user_id"`
	Currency            string    `json:"currency"`
	CurrentBalance      float64   `json:"current_balance"`
	TotalCredits        float64   `json:"total_credits"`
	TotalDebits         float64   `json:"total_debits"`
	TransactionCount    int64     `json:"transaction_count"`
	LastTransactionAt   time.Time `json:"last_transaction_at"`
	CreatedAt           time.Time `json:"created_at"`
	LastUpdated         time.Time `json:"last_updated"`
	LastProcessedEvent  int64     `json:"last_processed_event"`
}

// PaymentStatusReadModel representa el estado de pagos por usuario
type PaymentStatusReadModel struct {
	UserID            string    `json:"user_id"`
	TotalPayments     int64     `json:"total_payments"`
	CompletedPayments int64     `json:"completed_payments"`
	FailedPayments    int64     `json:"failed_payments"`
	PendingPayments   int64     `json:"pending_payments"`
	TotalAmount       float64   `json:"total_amount"`
	LastPaymentAt     time.Time `json:"last_payment_at"`
	LastUpdated       time.Time `json:"last_updated"`
}

// ReadModelProjector maneja la proyección de eventos a read models
type ReadModelProjector struct {
	store       ReadModelStore
	eventStore  eventstore.EventStore
	
	// Configuración
	batchSize   int
	maxRetries  int
	
	// Estado interno
	mu              sync.RWMutex
	lastProcessed   int64
	isRunning       bool
	stopChan        chan struct{}
	
	// Manejadores de eventos
	eventHandlers map[string]EventHandler
}

// EventHandler define una función que procesa un evento específico
type EventHandler func(ctx context.Context, event *eventstore.Event, projector *ReadModelProjector) error

// NewReadModelProjector crea un nuevo proyector de read models
func NewReadModelProjector(store ReadModelStore, eventStore eventstore.EventStore) *ReadModelProjector {
	return &ReadModelProjector{
		store:         store,
		eventStore:    eventStore,
		batchSize:     100,
		maxRetries:    3,
		stopChan:      make(chan struct{}),
		eventHandlers: make(map[string]EventHandler),
	}
}

// RegisterHandler registra un manejador para un tipo de evento específico
func (p *ReadModelProjector) RegisterHandler(eventType string, handler EventHandler) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.eventHandlers[eventType] = handler
}

// Start inicia el proyector para procesar eventos
func (p *ReadModelProjector) Start(ctx context.Context) error {
	p.mu.Lock()
	if p.isRunning {
		p.mu.Unlock()
		return fmt.Errorf("projector is already running")
	}
	p.isRunning = true
	p.mu.Unlock()

	// Obtener último evento procesado
	if err := p.loadCheckpoint(ctx); err != nil {
		return fmt.Errorf("failed to load checkpoint: %w", err)
	}

	// Procesar eventos existentes (catch-up)
	if err := p.catchUp(ctx); err != nil {
		return fmt.Errorf("failed to catch up: %w", err)
	}

	// Suscribirse a nuevos eventos
	eventChan, err := p.eventStore.Subscribe(ctx, eventstore.EventFilter{
		FromSequenceNumber: &p.lastProcessed,
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to events: %w", err)
	}

	// Procesar eventos en tiempo real
	go func() {
		defer func() {
			p.mu.Lock()
			p.isRunning = false
			p.mu.Unlock()
		}()

		for {
			select {
			case <-ctx.Done():
				return
			case <-p.stopChan:
				return
			case event := <-eventChan:
				if event != nil {
					if err := p.processEvent(ctx, event); err != nil {
						// En producción, esto debería loggearse apropiadamente
						fmt.Printf("Error processing event %s: %v\n", event.ID, err)
					}
				}
			}
		}
	}()

	return nil
}

// Stop detiene el proyector
func (p *ReadModelProjector) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	if p.isRunning {
		close(p.stopChan)
		p.isRunning = false
	}
}

// catchUp procesa todos los eventos desde el último checkpoint
func (p *ReadModelProjector) catchUp(ctx context.Context) error {
	// Get all events that haven't been processed yet
	filter := eventstore.EventFilter{}
	if p.lastProcessed > 0 {
		fromSeq := p.lastProcessed + 1
		filter.FromSequenceNumber = &fromSeq
	}
	
	events, err := p.eventStore.GetEvents(ctx, filter)
	if err != nil {
		return fmt.Errorf("failed to get events: %w", err)
	}
	
	fmt.Printf("CatchUp: Found %d events to process (lastProcessed: %d)\n", len(events), p.lastProcessed)
	
	// Process all events
	for _, event := range events {
		fmt.Printf("CatchUp: Processing event %s (seq: %d, type: %s)\n", event.ID, event.SequenceNumber, event.EventType)
		if err := p.processEvent(ctx, event); err != nil {
			return fmt.Errorf("failed to process event %s: %w", event.ID, err)
		}
		p.lastProcessed = event.SequenceNumber
	}
	
	fmt.Printf("CatchUp: Completed. LastProcessed: %d\n", p.lastProcessed)
	return nil
}

// processEvent procesa un evento individual
func (p *ReadModelProjector) processEvent(ctx context.Context, event *eventstore.Event) error {
	p.mu.RLock()
	handler, exists := p.eventHandlers[event.EventType]
	p.mu.RUnlock()
	
	if !exists {
		// Evento no manejado, simplemente actualizar checkpoint
		fmt.Printf("No handler for event type: %s\n", event.EventType)
		return p.updateCheckpoint(ctx, event.SequenceNumber)
	}
	
	fmt.Printf("Processing event: %s with handler\n", event.EventType)
	
	// Procesar evento con reintentos
	var err error
	for retry := 0; retry < p.maxRetries; retry++ {
		if err = handler(ctx, event, p); err == nil {
			fmt.Printf("Successfully processed event: %s\n", event.EventType)
			break
		}
		
		fmt.Printf("Error processing event %s (retry %d): %v\n", event.EventType, retry+1, err)
		// Backoff exponencial
		time.Sleep(time.Duration(retry+1) * 100 * time.Millisecond)
	}
	
	if err != nil {
		return fmt.Errorf("failed to process event after %d retries: %w", p.maxRetries, err)
	}
	
	// Actualizar checkpoint
	return p.updateCheckpoint(ctx, event.SequenceNumber)
}

// loadCheckpoint carga el último evento procesado
func (p *ReadModelProjector) loadCheckpoint(ctx context.Context) error {
	var checkpoint struct {
		LastProcessed int64 `json:"last_processed"`
	}
	
	err := p.store.Get(ctx, "projector:checkpoint", &checkpoint)
	if err != nil {
		// Si no existe checkpoint, empezar desde 0
		p.lastProcessed = 0
		return nil
	}
	
	p.lastProcessed = checkpoint.LastProcessed
	return nil
}

// updateCheckpoint actualiza el checkpoint del proyector
func (p *ReadModelProjector) updateCheckpoint(ctx context.Context, sequenceNumber int64) error {
	p.lastProcessed = sequenceNumber
	
	checkpoint := struct {
		LastProcessed int64     `json:"last_processed"`
		UpdatedAt     time.Time `json:"updated_at"`
	}{
		LastProcessed: sequenceNumber,
		UpdatedAt:     time.Now().UTC(),
	}
	
	return p.store.Set(ctx, "projector:checkpoint", checkpoint)
}

// BalanceProjectionService maneja las proyecciones de saldo
type BalanceProjectionService struct {
	projector *ReadModelProjector
	store     ReadModelStore
	logger    *log.Logger
}

// NewBalanceProjectionService crea un nuevo servicio de proyección de balances
func NewBalanceProjectionService(store ReadModelStore, eventStore eventstore.EventStore, logger *log.Logger) *BalanceProjectionService {
	projector := NewReadModelProjector(store, eventStore)
	
	service := &BalanceProjectionService{
		projector: projector,
		store:     store,
		logger:    logger,
	}
	
	// Registrar manejadores de eventos
	service.registerEventHandlers()
	
	return service
}

// registerEventHandlers registra los manejadores para eventos de billetera
func (s *BalanceProjectionService) registerEventHandlers() {
	// Evento: WalletDeducted
	s.projector.RegisterHandler("WalletDeducted", s.handleWalletDeducted)
	
	// Evento: WalletCredited
	s.projector.RegisterHandler("WalletCredited", s.handleWalletCredited)
	
	// Evento: WalletRefunded (para reembolsos)
	s.projector.RegisterHandler("WalletRefunded", s.handleWalletCredited) // Usar el mismo handler que WalletCredited
	
	// Evento: PaymentCompleted (para estadísticas)
	s.projector.RegisterHandler("PaymentCompleted", s.handlePaymentCompleted)
	
	// Evento: PaymentFailed (para estadísticas)
	s.projector.RegisterHandler("PaymentFailed", s.handlePaymentFailed)
	
	// Evento: PaymentInitiated (para estadísticas)
	s.projector.RegisterHandler("PaymentInitiated", s.handlePaymentInitiated)
}

// handleWalletDeducted maneja eventos de deducción de billetera
func (s *BalanceProjectionService) handleWalletDeducted(ctx context.Context, event *eventstore.Event, projector *ReadModelProjector) error {
	var eventData eventstore.WalletDeductedEventData
	if err := event.UnmarshalEventData(&eventData); err != nil {
		return fmt.Errorf("failed to unmarshal WalletDeducted event: %w", err)
	}
	
	// Actualizar balance read model
	balanceKey := fmt.Sprintf("balance:%s:%s", eventData.UserID, eventData.Currency)
	
	var balance BalanceReadModel
	err := s.store.Get(ctx, balanceKey, &balance)
	if err != nil {
		// Crear nuevo balance si no existe
		balance = BalanceReadModel{
			UserID:           eventData.UserID,
			Currency:         eventData.Currency,
			Balance:          0,
			TransactionCount: 0,
			Metadata:         make(map[string]interface{}),
		}
	}
	
	// Actualizar saldo
	balance.Balance = eventData.NewBalance
	balance.LastUpdated = event.CreatedAt
	balance.LastEventVersion = event.AggregateVersion
	balance.TransactionCount++
	
	// Guardar balance actualizado
	if err := s.store.Set(ctx, balanceKey, balance); err != nil {
		return fmt.Errorf("failed to update balance: %w", err)
	}
	
	// Actualizar wallet summary
	return s.updateWalletSummary(ctx, eventData.UserID, eventData.Currency, eventData.Amount, "debit", event.CreatedAt, event.SequenceNumber)
}

// handleWalletCredited maneja eventos de crédito de billetera
func (s *BalanceProjectionService) handleWalletCredited(ctx context.Context, event *eventstore.Event, projector *ReadModelProjector) error {
	var eventData eventstore.WalletCreditedEventData
	if err := event.UnmarshalEventData(&eventData); err != nil {
		return fmt.Errorf("failed to unmarshal WalletCredited event: %w", err)
	}
	
	fmt.Printf("Handler: Processing WalletCredited for UserID: %s, Currency: %s, Amount: %f\n", 
		eventData.UserID, eventData.Currency, eventData.NewBalance)
	
	// Actualizar balance read model
	balanceKey := fmt.Sprintf("balance:%s:%s", eventData.UserID, eventData.Currency)
	fmt.Printf("Handler: Using balance key: %s\n", balanceKey)
	
	var balance BalanceReadModel
	err := s.store.Get(ctx, balanceKey, &balance)
	if err != nil {
		fmt.Printf("Handler: Creating new balance (error: %v)\n", err)
		balance = BalanceReadModel{
			UserID:           eventData.UserID,
			Currency:         eventData.Currency,
			Balance:          0,
			TransactionCount: 0,
			Metadata:         make(map[string]interface{}),
		}
	} else {
		fmt.Printf("Handler: Found existing balance: %+v\n", balance)
	}
	
	balance.Balance = eventData.NewBalance
	balance.LastUpdated = event.CreatedAt
	balance.LastEventVersion = event.AggregateVersion
	balance.TransactionCount++
	
	fmt.Printf("Handler: Saving balance: %+v\n", balance)
	if err := s.store.Set(ctx, balanceKey, balance); err != nil {
		fmt.Printf("Handler: Failed to save balance: %v\n", err)
		return fmt.Errorf("failed to update balance: %w", err)
	}
	fmt.Printf("Handler: Successfully saved balance for key: %s\n", balanceKey)
	
	return s.updateWalletSummary(ctx, eventData.UserID, eventData.Currency, eventData.Amount, "credit", event.CreatedAt, event.SequenceNumber)
}

// handlePaymentInitiated maneja eventos de pago iniciado
func (s *BalanceProjectionService) handlePaymentInitiated(ctx context.Context, event *eventstore.Event, projector *ReadModelProjector) error {
	var eventData eventstore.PaymentInitiatedEventData
	if err := event.UnmarshalEventData(&eventData); err != nil {
		return fmt.Errorf("failed to unmarshal PaymentInitiated event: %w", err)
	}
	
	return s.updatePaymentStatus(ctx, eventData.UserID, "initiated", eventData.Amount, event.CreatedAt)
}

// handlePaymentCompleted maneja eventos de pago completado
func (s *BalanceProjectionService) handlePaymentCompleted(ctx context.Context, event *eventstore.Event, projector *ReadModelProjector) error {
	var eventData eventstore.PaymentCompletedEventData
	if err := event.UnmarshalEventData(&eventData); err != nil {
		return fmt.Errorf("failed to unmarshal PaymentCompleted event: %w", err)
	}
	
	// Obtener información del pago desde el agregado
	// En una implementación real, podríamos tener el userID en el evento
	// Por ahora, extraerlo del aggregate_id o usar metadatos
	userID := event.CausedBy // Asumiendo que CausedBy contiene el userID
	
	return s.updatePaymentStatus(ctx, userID, "completed", 0, event.CreatedAt)
}

// handlePaymentFailed maneja eventos de pago fallido
func (s *BalanceProjectionService) handlePaymentFailed(ctx context.Context, event *eventstore.Event, projector *ReadModelProjector) error {
	var eventData eventstore.PaymentFailedEventData
	if err := event.UnmarshalEventData(&eventData); err != nil {
		return fmt.Errorf("failed to unmarshal PaymentFailed event: %w", err)
	}
	
	userID := event.CausedBy
	
	return s.updatePaymentStatus(ctx, userID, "failed", 0, event.CreatedAt)
}

// updatePaymentStatus actualiza las estadísticas de pagos
func (s *BalanceProjectionService) updatePaymentStatus(ctx context.Context, userID, status string, amount float64, timestamp time.Time) error {
	statusKey := fmt.Sprintf("payment_status:%s", userID)
	
	var paymentStatus PaymentStatusReadModel
	err := s.store.Get(ctx, statusKey, &paymentStatus)
	if err != nil {
		paymentStatus = PaymentStatusReadModel{
			UserID: userID,
		}
	}
	
	switch status {
	case "initiated":
		paymentStatus.TotalPayments++
		paymentStatus.PendingPayments++
		paymentStatus.TotalAmount += amount
	case "completed":
		paymentStatus.CompletedPayments++
		if paymentStatus.PendingPayments > 0 {
			paymentStatus.PendingPayments--
		}
	case "failed":
		paymentStatus.FailedPayments++
		if paymentStatus.PendingPayments > 0 {
			paymentStatus.PendingPayments--
		}
	}
	
	paymentStatus.LastPaymentAt = timestamp
	paymentStatus.LastUpdated = time.Now().UTC()
	
	return s.store.Set(ctx, statusKey, paymentStatus)
}

// updateWalletSummary actualiza el resumen de la billetera
func (s *BalanceProjectionService) updateWalletSummary(ctx context.Context, userID, currency string, amount float64, transactionType string, timestamp time.Time, sequenceNumber int64) error {
	summaryKey := fmt.Sprintf("wallet_summary:%s:%s", userID, currency)
	
	var summary WalletSummaryReadModel
	err := s.store.Get(ctx, summaryKey, &summary)
	if err != nil {
		summary = WalletSummaryReadModel{
			UserID:   userID,
			Currency: currency,
		}
	}
	
	switch transactionType {
	case "debit":
		summary.TotalDebits += amount
		summary.TransactionCount++
	case "credit":
		summary.TotalCredits += amount
		summary.TransactionCount++
	}
	
	// Get current balance from balance read model
	balanceKey := fmt.Sprintf("balance:%s:%s", userID, currency)
	var balance BalanceReadModel
	if err := s.store.Get(ctx, balanceKey, &balance); err == nil {
		summary.CurrentBalance = balance.Balance
	}
	
	summary.LastTransactionAt = timestamp
	summary.LastUpdated = time.Now().UTC()
	summary.LastProcessedEvent = sequenceNumber
	
	return s.store.Set(ctx, summaryKey, summary)
}

// Start inicia el servicio de proyección
func (s *BalanceProjectionService) Start(ctx context.Context) error {
	return s.projector.Start(ctx)
}

// Stop detiene el servicio de proyección
func (s *BalanceProjectionService) Stop() {
	s.projector.Stop()
}

// GetBalance retorna el saldo actual de una billetera
func (s *BalanceProjectionService) GetBalance(ctx context.Context, userID, currency string) (*BalanceReadModel, error) {
	balanceKey := fmt.Sprintf("balance:%s:%s", userID, currency)
	
	var balance BalanceReadModel
	err := s.store.Get(ctx, balanceKey, &balance)
	if err != nil {
		// Retornar balance cero si no existe
		return &BalanceReadModel{
			UserID:           userID,
			Currency:         currency,
			Balance:          0,
			LastUpdated:      time.Now().UTC(),
			TransactionCount: 0,
			Metadata:         make(map[string]interface{}),
		}, nil
	}
	
	return &balance, nil
}

// GetWalletSummary retorna el resumen de una billetera
func (s *BalanceProjectionService) GetWalletSummary(ctx context.Context, userID, currency string) (*WalletSummaryReadModel, error) {
	summaryKey := fmt.Sprintf("wallet_summary:%s:%s", userID, currency)
	
	var summary WalletSummaryReadModel
	err := s.store.Get(ctx, summaryKey, &summary)
	if err != nil {
		return nil, fmt.Errorf("wallet summary not found for user %s currency %s", userID, currency)
	}
	
	return &summary, nil
}

// GetPaymentStatus retorna las estadísticas de pagos de un usuario
func (s *BalanceProjectionService) GetPaymentStatus(ctx context.Context, userID string) (*PaymentStatusReadModel, error) {
	statusKey := fmt.Sprintf("payment_status:%s", userID)
	
	var status PaymentStatusReadModel
	err := s.store.Get(ctx, statusKey, &status)
	if err != nil {
		return &PaymentStatusReadModel{
			UserID: userID,
		}, nil
	}
	
	return &status, nil
}

// GetAllBalances retorna todos los saldos (útil para administración)
func (s *BalanceProjectionService) GetAllBalances(ctx context.Context) (map[string]*BalanceReadModel, error) {
	allData, err := s.store.GetAll(ctx, "balance:*")
	if err != nil {
		return nil, fmt.Errorf("failed to get all balances: %w", err)
	}
	
	result := make(map[string]*BalanceReadModel)
	for key, value := range allData {
		var balance BalanceReadModel
		data, err := json.Marshal(value)
		if err != nil {
			continue
		}
		
		if err := json.Unmarshal(data, &balance); err != nil {
			continue
		}
		
		result[key] = &balance
	}
	
	return result, nil
}

// Estructuras de datos para eventos (complementar las existentes)
type WalletRefundedEventData struct {
	TransactionID   string    `json:"transaction_id"`
	Amount          float64   `json:"amount"`
	PreviousBalance float64   `json:"previous_balance"`
	NewBalance      float64   `json:"new_balance"`
	RefundReason    string    `json:"refund_reason"`
	OriginalPaymentID string  `json:"original_payment_id"`
	RefundedAt      time.Time `json:"refunded_at"`
	UserID          string    `json:"user_id"`
	Currency        string    `json:"currency"`
}
