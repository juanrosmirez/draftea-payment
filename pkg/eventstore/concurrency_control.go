package eventstore

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// WalletServiceInterface defines the interface for wallet operations
type WalletServiceInterface interface {
	ValidateBalance(ctx context.Context, userID uuid.UUID, amount int64, currency string) error
	DeductBalance(ctx context.Context, userID uuid.UUID, amount int64, currency string, paymentID uuid.UUID) error
	RefundBalance(ctx context.Context, userID uuid.UUID, amount int64, currency string, paymentID uuid.UUID, reason string) error
	RefundFunds(ctx context.Context, userID string, currency string, amount float64, reason string) error
}

// ConcurrencyControlStrategy define las estrategias de control de concurrencia
type ConcurrencyControlStrategy string

const (
	PessimisticLocking ConcurrencyControlStrategy = "pessimistic"
	OptimisticLocking  ConcurrencyControlStrategy = "optimistic"
	PartitionBased     ConcurrencyControlStrategy = "partition_based"
)

// UserLockManager maneja locks por usuario para evitar condiciones de carrera
type UserLockManager struct {
	locks   map[string]*sync.RWMutex
	mu      sync.RWMutex
	timeout time.Duration
}

// NewUserLockManager crea un nuevo gestor de locks por usuario
func NewUserLockManager(timeout time.Duration) *UserLockManager {
	return &UserLockManager{
		locks:   make(map[string]*sync.RWMutex),
		timeout: timeout,
	}
}

// getLock obtiene o crea un lock para un usuario específico
func (ulm *UserLockManager) getLock(userID string) *sync.RWMutex {
	ulm.mu.RLock()
	if lock, exists := ulm.locks[userID]; exists {
		ulm.mu.RUnlock()
		return lock
	}
	ulm.mu.RUnlock()

	// Crear nuevo lock si no existe
	ulm.mu.Lock()
	defer ulm.mu.Unlock()

	// Double-check después de obtener write lock
	if lock, exists := ulm.locks[userID]; exists {
		return lock
	}

	// Crear nuevo lock
	lock := &sync.RWMutex{}
	ulm.locks[userID] = lock
	return lock
}

// LockUser bloquea las operaciones para un usuario específico
func (ulm *UserLockManager) LockUser(ctx context.Context, userID string) (*UserLock, error) {
	lock := ulm.getLock(userID)

	// Canal para señalar cuando se obtiene el lock
	lockAcquired := make(chan struct{})

	go func() {
		lock.Lock()
		close(lockAcquired)
	}()

	// Esperar lock con timeout
	select {
	case <-lockAcquired:
		return &UserLock{
			userID: userID,
			lock:   lock,
			ulm:    ulm,
		}, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("failed to acquire lock for user %s: %w", userID, ctx.Err())
	case <-time.After(ulm.timeout):
		return nil, fmt.Errorf("timeout acquiring lock for user %s after %v", userID, ulm.timeout)
	}
}

// RLockUser bloquea para lectura las operaciones para un usuario específico
func (ulm *UserLockManager) RLockUser(ctx context.Context, userID string) (*UserRLock, error) {
	lock := ulm.getLock(userID)

	lockAcquired := make(chan struct{})

	go func() {
		lock.RLock()
		close(lockAcquired)
	}()

	select {
	case <-lockAcquired:
		return &UserRLock{
			userID: userID,
			lock:   lock,
			ulm:    ulm,
		}, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("failed to acquire read lock for user %s: %w", userID, ctx.Err())
	case <-time.After(ulm.timeout):
		return nil, fmt.Errorf("timeout acquiring read lock for user %s after %v", userID, ulm.timeout)
	}
}

// UserLock representa un lock exclusivo para un usuario
type UserLock struct {
	userID string
	lock   *sync.RWMutex
	ulm    *UserLockManager
}

// Unlock libera el lock exclusivo
func (ul *UserLock) Unlock() {
	ul.lock.Unlock()
}

// UserRLock representa un lock de lectura para un usuario
type UserRLock struct {
	userID string
	lock   *sync.RWMutex
	ulm    *UserLockManager
}

// RUnlock libera el lock de lectura
func (url *UserRLock) RUnlock() {
	url.lock.RUnlock()
}

// ConcurrentWalletService implementa un servicio de billetera thread-safe
type ConcurrentWalletService struct {
	balances    map[string]map[string]*WalletBalance
	mu          sync.RWMutex
	lockManager *UserLockManager
	strategy    ConcurrencyControlStrategy
	logger      Logger
}

// WalletBalance representa el saldo de una billetera con control de versión
type WalletBalance struct {
	UserID           string       `json:"user_id"`
	Currency         string       `json:"currency"`
	Balance          float64      `json:"balance"`
	Version          int64        `json:"version"`
	LastUpdated      time.Time    `json:"last_updated"`
	TransactionCount int64        `json:"transaction_count"`
	mu               sync.RWMutex // Lock interno por balance
}

// NewConcurrentWalletService crea un nuevo servicio de billetera concurrente
func NewConcurrentWalletService(strategy ConcurrencyControlStrategy, logger Logger) *ConcurrentWalletService {
	return &ConcurrentWalletService{
		balances:    make(map[string]map[string]*WalletBalance),
		lockManager: NewUserLockManager(5 * time.Second),
		strategy:    strategy,
		logger:      logger,
	}
}

// getOrCreateBalance obtiene o crea un balance para usuario/moneda
func (cws *ConcurrentWalletService) getOrCreateBalance(userID, currency string) *WalletBalance {
	cws.mu.Lock()
	defer cws.mu.Unlock()

	if cws.balances[userID] == nil {
		cws.balances[userID] = make(map[string]*WalletBalance)
	}

	if cws.balances[userID][currency] == nil {
		cws.balances[userID][currency] = &WalletBalance{
			UserID:      userID,
			Currency:    currency,
			Balance:     0.0,
			Version:     1,
			LastUpdated: time.Now().UTC(),
		}
	}

	return cws.balances[userID][currency]
}

// CheckBalance obtiene el saldo actual (operación de lectura)
func (cws *ConcurrentWalletService) CheckBalance(ctx context.Context, userID, currency string) (float64, error) {
	switch cws.strategy {
	case PessimisticLocking:
		return cws.checkBalancePessimistic(ctx, userID, currency)
	case OptimisticLocking:
		return cws.checkBalanceOptimistic(ctx, userID, currency)
	case PartitionBased:
		return cws.checkBalancePartitioned(ctx, userID, currency)
	default:
		return 0, fmt.Errorf("unknown concurrency strategy: %s", cws.strategy)
	}
}

// checkBalancePessimistic usa locks para lectura
func (cws *ConcurrentWalletService) checkBalancePessimistic(ctx context.Context, userID, currency string) (float64, error) {
	// Obtener read lock para el usuario
	userLock, err := cws.lockManager.RLockUser(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("failed to acquire read lock: %w", err)
	}
	defer userLock.RUnlock()

	cws.logger.Info("Reading balance with pessimistic lock",
		"user_id", userID,
		"currency", currency,
	)

	balance := cws.getOrCreateBalance(userID, currency)

	balance.mu.RLock()
	defer balance.mu.RUnlock()

	return balance.Balance, nil
}

// checkBalanceOptimistic lee sin locks (eventual consistency)
func (cws *ConcurrentWalletService) checkBalanceOptimistic(ctx context.Context, userID, currency string) (float64, error) {
	_ = ctx // Context not used in this optimistic read but kept for interface consistency
	balance := cws.getOrCreateBalance(userID, currency)

	balance.mu.RLock()
	defer balance.mu.RUnlock()

	cws.logger.Info("Reading balance with optimistic approach",
		"user_id", userID,
		"currency", currency,
		"version", balance.Version,
	)

	return balance.Balance, nil
}

// checkBalancePartitioned simula lectura desde partición específica
func (cws *ConcurrentWalletService) checkBalancePartitioned(ctx context.Context, userID, currency string) (float64, error) {
	_ = ctx // Context not used in this partition simulation but kept for interface consistency
	// En un sistema real, esto sería enrutado a la partición correcta
	partitionID := cws.getPartitionForUser(userID)

	cws.logger.Info("Reading balance from partition",
		"user_id", userID,
		"currency", currency,
		"partition_id", partitionID,
	)

	balance := cws.getOrCreateBalance(userID, currency)

	balance.mu.RLock()
	defer balance.mu.RUnlock()

	return balance.Balance, nil
}

// DeductFunds deduce fondos con control de concurrencia
func (cws *ConcurrentWalletService) DeductFunds(ctx context.Context, userID, currency string, amount float64, reason string) error {
	switch cws.strategy {
	case PessimisticLocking:
		return cws.deductFundsPessimistic(ctx, userID, currency, amount, reason)
	case OptimisticLocking:
		return cws.deductFundsOptimistic(ctx, userID, currency, amount, reason)
	case PartitionBased:
		return cws.deductFundsPartitioned(ctx, userID, currency, amount, reason)
	default:
		return fmt.Errorf("unknown concurrency strategy: %s", cws.strategy)
	}
}

// deductFundsPessimistic usa locks exclusivos para escritura
func (cws *ConcurrentWalletService) deductFundsPessimistic(ctx context.Context, userID, currency string, amount float64, reason string) error {
	// Obtener lock exclusivo para el usuario
	userLock, err := cws.lockManager.LockUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to acquire exclusive lock: %w", err)
	}
	defer userLock.Unlock()

	cws.logger.Info("Deducting funds with pessimistic lock",
		"user_id", userID,
		"currency", currency,
		"amount", amount,
		"reason", reason,
	)

	balance := cws.getOrCreateBalance(userID, currency)

	balance.mu.Lock()
	defer balance.mu.Unlock()

	// Verificar saldo suficiente
	if balance.Balance < amount {
		return fmt.Errorf("insufficient balance: have %.2f, need %.2f", balance.Balance, amount)
	}

	// Actualizar saldo
	previousBalance := balance.Balance
	balance.Balance -= amount
	balance.Version++
	balance.LastUpdated = time.Now().UTC()
	balance.TransactionCount++

	cws.logger.Info("Funds deducted successfully",
		"user_id", userID,
		"currency", currency,
		"amount", amount,
		"previous_balance", previousBalance,
		"new_balance", balance.Balance,
		"version", balance.Version,
	)

	return nil
}

// deductFundsOptimistic usa control optimista con versioning
func (cws *ConcurrentWalletService) deductFundsOptimistic(ctx context.Context, userID, currency string, amount float64, reason string) error {
	_ = ctx    // Context not used in this optimistic implementation but kept for interface consistency
	_ = reason // Reason not logged in this implementation but kept for interface consistency
	maxRetries := 5

	for attempt := 0; attempt < maxRetries; attempt++ {
		balance := cws.getOrCreateBalance(userID, currency)

		// Leer estado actual
		balance.mu.RLock()
		currentVersion := balance.Version
		currentBalance := balance.Balance
		balance.mu.RUnlock()

		cws.logger.Info("Attempting optimistic deduction",
			"user_id", userID,
			"currency", currency,
			"amount", amount,
			"attempt", attempt+1,
			"current_version", currentVersion,
			"current_balance", currentBalance,
		)

		// Verificar saldo suficiente
		if currentBalance < amount {
			return fmt.Errorf("insufficient balance: have %.2f, need %.2f", currentBalance, amount)
		}

		// Intentar actualización atómica
		balance.mu.Lock()

		// Verificar que la versión no haya cambiado (optimistic concurrency control)
		if balance.Version != currentVersion {
			balance.mu.Unlock()
			cws.logger.Warn("Version conflict detected, retrying",
				"user_id", userID,
				"expected_version", currentVersion,
				"actual_version", balance.Version,
				"attempt", attempt+1,
			)

			// Backoff exponencial
			time.Sleep(time.Duration(attempt+1) * 10 * time.Millisecond)
			continue
		}

		// Actualizar saldo
		previousBalance := balance.Balance
		balance.Balance -= amount
		balance.Version++
		balance.LastUpdated = time.Now().UTC()
		balance.TransactionCount++

		balance.mu.Unlock()

		cws.logger.Info("Optimistic deduction successful",
			"user_id", userID,
			"currency", currency,
			"amount", amount,
			"previous_balance", previousBalance,
			"new_balance", balance.Balance,
			"new_version", balance.Version,
			"attempts", attempt+1,
		)

		return nil
	}

	return fmt.Errorf("failed to deduct funds after %d attempts due to version conflicts", maxRetries)
}

// deductFundsPartitioned simula procesamiento en partición específica
func (cws *ConcurrentWalletService) deductFundsPartitioned(ctx context.Context, userID, currency string, amount float64, reason string) error {
	_ = ctx // Context not used in this partition simulation but kept for interface consistency
	partitionID := cws.getPartitionForUser(userID)

	cws.logger.Info("Processing deduction in partition",
		"user_id", userID,
		"currency", currency,
		"amount", amount,
		"partition_id", partitionID,
		"reason", reason,
	)

	// En un sistema particionado, todas las operaciones del mismo usuario
	// se procesan secuencialmente en la misma partición
	balance := cws.getOrCreateBalance(userID, currency)

	balance.mu.Lock()
	defer balance.mu.Unlock()

	if balance.Balance < amount {
		return fmt.Errorf("insufficient balance: have %.2f, need %.2f", balance.Balance, amount)
	}

	previousBalance := balance.Balance
	balance.Balance -= amount
	balance.Version++
	balance.LastUpdated = time.Now().UTC()
	balance.TransactionCount++

	cws.logger.Info("Partitioned deduction successful",
		"user_id", userID,
		"partition_id", partitionID,
		"previous_balance", previousBalance,
		"new_balance", balance.Balance,
	)

	return nil
}

// RefundFunds agrega fondos con control de concurrencia
func (cws *ConcurrentWalletService) RefundFunds(ctx context.Context, userID, currency string, amount float64, reason string) error {
	switch cws.strategy {
	case PessimisticLocking:
		return cws.refundFundsPessimistic(ctx, userID, currency, amount, reason)
	case OptimisticLocking:
		return cws.refundFundsOptimistic(ctx, userID, currency, amount, reason)
	case PartitionBased:
		return cws.refundFundsPartitioned(ctx, userID, currency, amount, reason)
	default:
		return fmt.Errorf("unknown concurrency strategy: %s", cws.strategy)
	}
}

// refundFundsPessimistic usa locks exclusivos
func (cws *ConcurrentWalletService) refundFundsPessimistic(ctx context.Context, userID, currency string, amount float64, reason string) error {
	userLock, err := cws.lockManager.LockUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to acquire exclusive lock: %w", err)
	}
	defer userLock.Unlock()

	balance := cws.getOrCreateBalance(userID, currency)

	balance.mu.Lock()
	defer balance.mu.Unlock()

	previousBalance := balance.Balance
	balance.Balance += amount
	balance.Version++
	balance.LastUpdated = time.Now().UTC()
	balance.TransactionCount++

	cws.logger.Info("Pessimistic refund successful",
		"user_id", userID,
		"currency", currency,
		"amount", amount,
		"previous_balance", previousBalance,
		"new_balance", balance.Balance,
		"reason", reason,
	)

	return nil
}

// refundFundsOptimistic usa control optimista
func (cws *ConcurrentWalletService) refundFundsOptimistic(ctx context.Context, userID, currency string, amount float64, reason string) error {
	_ = ctx    // Context not used in this optimistic implementation but kept for interface consistency
	_ = reason // Reason not logged in this implementation but kept for interface consistency
	maxRetries := 5

	for attempt := 0; attempt < maxRetries; attempt++ {
		balance := cws.getOrCreateBalance(userID, currency)

		balance.mu.RLock()
		currentVersion := balance.Version
		balance.mu.RUnlock()

		balance.mu.Lock()

		if balance.Version != currentVersion {
			balance.mu.Unlock()
			time.Sleep(time.Duration(attempt+1) * 10 * time.Millisecond)
			continue
		}

		previousBalance := balance.Balance
		balance.Balance += amount
		balance.Version++
		balance.LastUpdated = time.Now().UTC()
		balance.TransactionCount++

		balance.mu.Unlock()

		cws.logger.Info("Optimistic refund successful",
			"user_id", userID,
			"currency", currency,
			"amount", amount,
			"previous_balance", previousBalance,
			"new_balance", balance.Balance,
			"attempts", attempt+1,
		)

		return nil
	}

	return fmt.Errorf("failed to refund funds after %d attempts due to version conflicts", maxRetries)
}

// refundFundsPartitioned procesa en partición específica
func (cws *ConcurrentWalletService) refundFundsPartitioned(ctx context.Context, userID, currency string, amount float64, reason string) error {
	_ = ctx    // Context not used in this partition simulation but kept for interface consistency
	_ = reason // Reason not logged in this implementation but kept for interface consistency
	partitionID := cws.getPartitionForUser(userID)

	balance := cws.getOrCreateBalance(userID, currency)

	balance.mu.Lock()
	defer balance.mu.Unlock()

	previousBalance := balance.Balance
	balance.Balance += amount
	balance.Version++
	balance.LastUpdated = time.Now().UTC()
	balance.TransactionCount++

	cws.logger.Info("Partitioned refund successful",
		"user_id", userID,
		"partition_id", partitionID,
		"previous_balance", previousBalance,
		"new_balance", balance.Balance,
	)

	return nil
}

// SetBalance establece un saldo específico (para testing)
func (cws *ConcurrentWalletService) SetBalance(userID, currency string, amount float64) {
	balance := cws.getOrCreateBalance(userID, currency)

	balance.mu.Lock()
	defer balance.mu.Unlock()

	balance.Balance = amount
	balance.Version++
	balance.LastUpdated = time.Now().UTC()
}

// GetBalanceWithVersion retorna saldo y versión para control optimista
func (cws *ConcurrentWalletService) GetBalanceWithVersion(ctx context.Context, userID, currency string) (float64, int64, error) {
	balance := cws.getOrCreateBalance(userID, currency)

	balance.mu.RLock()
	defer balance.mu.RUnlock()

	return balance.Balance, balance.Version, nil
}

// getPartitionForUser calcula la partición para un usuario
func (cws *ConcurrentWalletService) getPartitionForUser(userID string) int {
	// Hash simple para determinar partición
	hash := 0
	for _, char := range userID {
		hash = hash*31 + int(char)
	}

	// Simular 4 particiones
	return (hash % 4) + 1
}

// PartitionedEventProcessor procesa eventos por partición de usuario
type PartitionedEventProcessor struct {
	partitions map[int]*PartitionWorker
	mu         sync.RWMutex
	logger     Logger
}

// PartitionWorker procesa eventos para una partición específica
type PartitionWorker struct {
	partitionID   int
	eventChan     chan *Event
	walletService WalletServiceInterface
	logger        Logger
	stopChan      chan struct{}
}

// NewPartitionedEventProcessor crea un procesador de eventos particionado
func NewPartitionedEventProcessor(numPartitions int, walletService WalletServiceInterface, logger Logger) *PartitionedEventProcessor {
	processor := &PartitionedEventProcessor{
		partitions: make(map[int]*PartitionWorker),
		logger:     logger,
	}

	// Crear workers para cada partición
	for i := 1; i <= numPartitions; i++ {
		worker := &PartitionWorker{
			partitionID:   i,
			eventChan:     make(chan *Event, 100),
			walletService: walletService,
			logger:        logger,
			stopChan:      make(chan struct{}),
		}

		processor.partitions[i] = worker

		// Iniciar worker
		go worker.start()
	}

	return processor
}

// ProcessEvent enruta un evento a la partición correcta
func (pep *PartitionedEventProcessor) ProcessEvent(event *Event) error {
	// Extraer userID del evento
	userID := event.CausedBy
	if userID == "" {
		return fmt.Errorf("event missing userID for partitioning")
	}

	// Calcular partición
	partitionID := pep.getPartitionForUser(userID)

	pep.mu.RLock()
	worker, exists := pep.partitions[partitionID]
	pep.mu.RUnlock()

	if !exists {
		return fmt.Errorf("partition %d not found", partitionID)
	}

	// Enviar evento a la partición (procesamiento secuencial garantizado)
	select {
	case worker.eventChan <- event:
		pep.logger.Info("Event routed to partition",
			"event_id", event.ID,
			"user_id", userID,
			"partition_id", partitionID,
		)
		return nil
	default:
		return fmt.Errorf("partition %d event channel full", partitionID)
	}
}

// start inicia el procesamiento de eventos en la partición
func (pw *PartitionWorker) start() {
	pw.logger.Info("Starting partition worker", "partition_id", pw.partitionID)

	for {
		select {
		case event := <-pw.eventChan:
			pw.processEvent(event)
		case <-pw.stopChan:
			pw.logger.Info("Stopping partition worker", "partition_id", pw.partitionID)
			return
		}
	}
}

// processEvent procesa un evento específico
func (pw *PartitionWorker) processEvent(event *Event) {
	pw.logger.Info("Processing event in partition",
		"partition_id", pw.partitionID,
		"event_id", event.ID,
		"event_type", event.EventType,
		"user_id", event.CausedBy,
	)

	// Simular procesamiento del evento
	// En un sistema real, aquí se aplicarían las reglas de negocio
	time.Sleep(10 * time.Millisecond)

	pw.logger.Info("Event processed successfully",
		"partition_id", pw.partitionID,
		"event_id", event.ID,
	)
}

// getPartitionForUser calcula la partición para un usuario
func (pep *PartitionedEventProcessor) getPartitionForUser(userID string) int {
	hash := 0
	for _, char := range userID {
		hash = hash*31 + int(char)
	}

	numPartitions := len(pep.partitions)
	return (hash % numPartitions) + 1
}

// Stop detiene todos los workers
func (pep *PartitionedEventProcessor) Stop() {
	pep.mu.RLock()
	defer pep.mu.RUnlock()

	for _, worker := range pep.partitions {
		close(worker.stopChan)
	}
}

// GetBalance obtiene el saldo actual de una billetera
func (cws *ConcurrentWalletService) GetBalance(userID, currency string) (float64, error) {
	balance := cws.getOrCreateBalance(userID, currency)
	
	balance.mu.RLock()
	defer balance.mu.RUnlock()
	
	return balance.Balance, nil
}

// ProcessPaymentRequested procesa una solicitud de pago
func (cws *ConcurrentWalletService) ProcessPaymentRequested(ctx context.Context, userID, currency string, amount float64, paymentID string) error {
	switch cws.strategy {
	case OptimisticLocking:
		return cws.deductFundsOptimistic(ctx, userID, currency, amount, paymentID)
	case PessimisticLocking:
		return cws.deductFundsPessimistic(ctx, userID, currency, amount, paymentID)
	default:
		return fmt.Errorf("unknown concurrency control strategy: %v", cws.strategy)
	}
}
