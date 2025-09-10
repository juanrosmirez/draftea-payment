package wallet

import (
	"context"
	"fmt"
	"time"

	"payment-system/internal/events"
	
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Wallet entidad de billetera
type Wallet struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	Balance   int64     `json:"balance"`
	Currency  string    `json:"currency"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Version   int       `json:"version"`
}

// Transaction entidad de transacción
type Transaction struct {
	ID            uuid.UUID         `json:"id"`
	WalletID      uuid.UUID         `json:"wallet_id"`
	PaymentID     uuid.UUID         `json:"payment_id"`
	Type          TransactionType   `json:"type"`
	Amount        int64             `json:"amount"`
	Currency      string            `json:"currency"`
	Status        TransactionStatus `json:"status"`
	Description   string            `json:"description"`
	CreatedAt     time.Time         `json:"created_at"`
	CorrelationID uuid.UUID         `json:"correlation_id"`
}

// TransactionType tipos de transacción
type TransactionType string

const (
	TypeDebit  TransactionType = "debit"
	TypeCredit TransactionType = "credit"
)

// TransactionStatus estados de transacción
type TransactionStatus string

const (
	TxnStatusPending   TransactionStatus = "pending"
	TxnStatusCompleted TransactionStatus = "completed"
	TxnStatusFailed    TransactionStatus = "failed"
	TxnStatusReversed  TransactionStatus = "reversed"
)

// Repository interfaz para persistencia de billeteras
type Repository interface {
	GetWalletByUserID(ctx context.Context, userID uuid.UUID, currency string) (*Wallet, error)
	UpdateBalance(ctx context.Context, walletID uuid.UUID, newBalance int64, version int) error
	CreateTransaction(ctx context.Context, txn *Transaction) error
	GetTransactionsByWallet(ctx context.Context, walletID uuid.UUID) ([]*Transaction, error)
}

// Service servicio de billetera
type Service struct {
	repo           Repository
	eventPublisher events.Publisher
	logger         *zap.Logger
}

// NewService crea una nueva instancia del servicio de billetera
func NewService(repo Repository, eventPublisher events.Publisher, logger *zap.Logger) *Service {
	return &Service{
		repo:           repo,
		eventPublisher: eventPublisher,
		logger:         logger,
	}
}

// ValidateBalance valida si el usuario tiene suficiente saldo
func (s *Service) ValidateBalance(ctx context.Context, userID uuid.UUID, amount int64, currency string) error {
	wallet, err := s.repo.GetWalletByUserID(ctx, userID, currency)
	if err != nil {
		s.logger.Error("failed to get wallet", 
			zap.String("user_id", userID.String()),
			zap.String("currency", currency),
			zap.Error(err))
		return fmt.Errorf("failed to get wallet: %w", err)
	}

	if wallet.Balance < amount {
		s.logger.Warn("insufficient balance",
			zap.String("user_id", userID.String()),
			zap.Int64("required", amount),
			zap.Int64("available", wallet.Balance))
		
		// Publicar evento de fondos insuficientes
		insufficientEvent := events.BaseEvent{
			ID:          uuid.New(),
			Type:        events.WalletInsufficient,
			AggregateID: wallet.ID,
			Version:     wallet.Version + 1,
			Timestamp:   time.Now(),
			Metadata: map[string]interface{}{
				"user_id":   userID.String(),
				"required":  amount,
				"available": wallet.Balance,
				"currency":  currency,
			},
		}

		s.eventPublisher.Publish(ctx, insufficientEvent)
		return fmt.Errorf("insufficient balance: required %d, available %d", amount, wallet.Balance)
	}

	// Publicar evento de validación exitosa
	validatedEvent := events.BaseEvent{
		ID:          uuid.New(),
		Type:        events.WalletValidated,
		AggregateID: wallet.ID,
		Version:     wallet.Version + 1,
		Timestamp:   time.Now(),
		Metadata: map[string]interface{}{
			"user_id":  userID.String(),
			"amount":   amount,
			"currency": currency,
		},
	}

	s.eventPublisher.Publish(ctx, validatedEvent)
	return nil
}

// DeductBalance deduce saldo de la billetera del usuario
func (s *Service) DeductBalance(ctx context.Context, userID uuid.UUID, amount int64, currency string, paymentID uuid.UUID) error {
	wallet, err := s.repo.GetWalletByUserID(ctx, userID, currency)
	if err != nil {
		return fmt.Errorf("failed to get wallet: %w", err)
	}

	if wallet.Balance < amount {
		return fmt.Errorf("insufficient balance")
	}

	previousBalance := wallet.Balance
	newBalance := wallet.Balance - amount

	// Crear transacción de débito
	txn := &Transaction{
		ID:            uuid.New(),
		WalletID:      wallet.ID,
		PaymentID:     paymentID,
		Type:          TypeDebit,
		Amount:        amount,
		Currency:      currency,
		Status:        TxnStatusPending,
		Description:   fmt.Sprintf("Payment deduction for payment %s", paymentID.String()),
		CreatedAt:     time.Now(),
		CorrelationID: uuid.New(),
	}

	// Guardar transacción
	if err := s.repo.CreateTransaction(ctx, txn); err != nil {
		s.logger.Error("failed to create debit transaction", zap.Error(err))
		return fmt.Errorf("failed to create transaction: %w", err)
	}

	// Actualizar saldo de billetera
	if err := s.repo.UpdateBalance(ctx, wallet.ID, newBalance, wallet.Version); err != nil {
		s.logger.Error("failed to update wallet balance", zap.Error(err))
		return fmt.Errorf("failed to update balance: %w", err)
	}

	// Publicar evento de deducción
	deductedEvent := events.WalletDeductedEvent{
		BaseEvent: events.BaseEvent{
			ID:            uuid.New(),
			Type:          events.WalletDeducted,
			AggregateID:   wallet.ID,
			Version:       wallet.Version + 1,
			Timestamp:     time.Now(),
			CorrelationID: txn.CorrelationID,
		},
		UserID:          userID,
		Amount:          amount,
		Currency:        currency,
		PreviousBalance: previousBalance,
		NewBalance:      newBalance,
		PaymentID:       paymentID,
	}

	if err := s.eventPublisher.Publish(ctx, deductedEvent); err != nil {
		s.logger.Error("failed to publish wallet deducted event", zap.Error(err))
	}

	s.logger.Info("balance deducted successfully",
		zap.String("user_id", userID.String()),
		zap.Int64("amount", amount),
		zap.Int64("new_balance", newBalance),
		zap.String("payment_id", paymentID.String()))

	return nil
}

// RefundBalance reembolsa saldo a la billetera del usuario
func (s *Service) RefundBalance(ctx context.Context, userID uuid.UUID, amount int64, currency string, paymentID uuid.UUID, reason string) error {
	wallet, err := s.repo.GetWalletByUserID(ctx, userID, currency)
	if err != nil {
		return fmt.Errorf("failed to get wallet: %w", err)
	}

	previousBalance := wallet.Balance
	newBalance := wallet.Balance + amount

	// Crear transacción de crédito
	txn := &Transaction{
		ID:            uuid.New(),
		WalletID:      wallet.ID,
		PaymentID:     paymentID,
		Type:          TypeCredit,
		Amount:        amount,
		Currency:      currency,
		Status:        TxnStatusPending,
		Description:   fmt.Sprintf("Refund for payment %s: %s", paymentID.String(), reason),
		CreatedAt:     time.Now(),
		CorrelationID: uuid.New(),
	}

	// Guardar transacción
	if err := s.repo.CreateTransaction(ctx, txn); err != nil {
		s.logger.Error("failed to create credit transaction", zap.Error(err))
		return fmt.Errorf("failed to create transaction: %w", err)
	}

	// Actualizar saldo de billetera
	if err := s.repo.UpdateBalance(ctx, wallet.ID, newBalance, wallet.Version); err != nil {
		s.logger.Error("failed to update wallet balance", zap.Error(err))
		return fmt.Errorf("failed to update balance: %w", err)
	}

	// Publicar evento de reembolso
	refundedEvent := events.WalletRefundedEvent{
		BaseEvent: events.BaseEvent{
			ID:            uuid.New(),
			Type:          events.WalletRefunded,
			AggregateID:   wallet.ID,
			Version:       wallet.Version + 1,
			Timestamp:     time.Now(),
			CorrelationID: txn.CorrelationID,
		},
		UserID:          userID,
		Amount:          amount,
		Currency:        currency,
		PreviousBalance: previousBalance,
		NewBalance:      newBalance,
		PaymentID:       paymentID,
		Reason:          reason,
	}

	if err := s.eventPublisher.Publish(ctx, refundedEvent); err != nil {
		s.logger.Error("failed to publish wallet refunded event", zap.Error(err))
	}

	s.logger.Info("balance refunded successfully",
		zap.String("user_id", userID.String()),
		zap.Int64("amount", amount),
		zap.Int64("new_balance", newBalance),
		zap.String("payment_id", paymentID.String()),
		zap.String("reason", reason))

	return nil
}

// GetWallet obtiene la billetera de un usuario
func (s *Service) GetWallet(ctx context.Context, userID uuid.UUID, currency string) (*Wallet, error) {
	return s.repo.GetWalletByUserID(ctx, userID, currency)
}

// GetTransactionHistory obtiene el historial de transacciones
func (s *Service) GetTransactionHistory(ctx context.Context, userID uuid.UUID, currency string) ([]*Transaction, error) {
	wallet, err := s.repo.GetWalletByUserID(ctx, userID, currency)
	if err != nil {
		return nil, fmt.Errorf("failed to get wallet: %w", err)
	}

	return s.repo.GetTransactionsByWallet(ctx, wallet.ID)
}
