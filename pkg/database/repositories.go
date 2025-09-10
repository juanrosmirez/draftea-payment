package database

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"payment-system/internal/saga"
	"payment-system/internal/services/metrics"
	"payment-system/internal/services/payment"
	"payment-system/internal/services/wallet"

	"github.com/google/uuid"
)

// PaymentRepository implementación de repositorio de pagos
type PaymentRepository struct {
	db *DB
}

// NewPaymentRepository crea un nuevo repositorio de pagos
func NewPaymentRepository(db *DB) *PaymentRepository {
	return &PaymentRepository{db: db}
}

// Save guarda un pago en la base de datos
func (r *PaymentRepository) Save(ctx context.Context, p *payment.Payment) error {
	query := `
		INSERT INTO payments (id, user_id, amount, currency, service_id, description, status, gateway_txn_id, created_at, updated_at, correlation_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (id) DO UPDATE SET
			status = EXCLUDED.status,
			gateway_txn_id = EXCLUDED.gateway_txn_id,
			updated_at = EXCLUDED.updated_at
	`
	_, err := r.db.ExecContext(ctx, query,
		p.ID, p.UserID, p.Amount, p.Currency, p.ServiceID,
		p.Description, p.Status, p.GatewayTxnID, p.CreatedAt, p.UpdatedAt, p.CorrelationID)

	return err
}

// GetByID obtiene un pago por ID
func (r *PaymentRepository) GetByID(ctx context.Context, id uuid.UUID) (*payment.Payment, error) {
	query := `
		SELECT id, user_id, amount, currency, service_id, description, status, gateway_txn_id, created_at, updated_at, correlation_id
		FROM payments WHERE id = $1
	`

	p := &payment.Payment{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&p.ID, &p.UserID, &p.Amount, &p.Currency, &p.ServiceID,
		&p.Description, &p.Status, &p.GatewayTxnID, &p.CreatedAt, &p.UpdatedAt, &p.CorrelationID)

	if err != nil {
		return nil, err
	}

	return p, nil
}

// UpdateStatus actualiza el estado de un pago
func (r *PaymentRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status payment.PaymentStatus) error {
	query := `UPDATE payments SET status = $1, updated_at = $2 WHERE id = $3`
	_, err := r.db.ExecContext(ctx, query, status, time.Now(), id)
	return err
}

// WalletRepository implementación de repositorio de billeteras
type WalletRepository struct {
	db *DB
}

// NewWalletRepository crea un nuevo repositorio de billeteras
func NewWalletRepository(db *DB) *WalletRepository {
	return &WalletRepository{db: db}
}

// GetWalletByUserID obtiene una billetera por ID de usuario
func (r *WalletRepository) GetWalletByUserID(ctx context.Context, userID uuid.UUID, currency string) (*wallet.Wallet, error) {
	query := `
		SELECT id, user_id, balance, currency, created_at, updated_at, version
		FROM wallets WHERE user_id = $1 AND currency = $2
	`

	w := &wallet.Wallet{}
	err := r.db.QueryRowContext(ctx, query, userID, currency).Scan(
		&w.ID, &w.UserID, &w.Balance, &w.Currency, &w.CreatedAt, &w.UpdatedAt, &w.Version)

	if err != nil {
		return nil, err
	}

	return w, nil
}

// UpdateBalance actualiza el saldo de una billetera
func (r *WalletRepository) UpdateBalance(ctx context.Context, walletID uuid.UUID, newBalance int64, version int) error {
	query := `
		UPDATE wallets 
		SET balance = $1, updated_at = $2, version = version + 1 
		WHERE id = $3 AND version = $4
	`
	result, err := r.db.ExecContext(ctx, query, newBalance, time.Now(), walletID, version)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return fmt.Errorf("optimistic lock failed: wallet version mismatch")
	}

	return nil
}

// CreateTransaction crea una nueva transacción
func (r *WalletRepository) CreateTransaction(ctx context.Context, txn *wallet.Transaction) error {
	query := `
		INSERT INTO transactions (id, wallet_id, payment_id, type, amount, currency, status, description, created_at, correlation_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`
	_, err := r.db.ExecContext(ctx, query,
		txn.ID, txn.WalletID, txn.PaymentID, txn.Type, txn.Amount,
		txn.Currency, txn.Status, txn.Description, txn.CreatedAt, txn.CorrelationID)

	return err
}

// GetTransactionsByWallet obtiene transacciones por billetera
func (r *WalletRepository) GetTransactionsByWallet(ctx context.Context, walletID uuid.UUID) ([]*wallet.Transaction, error) {
	query := `
		SELECT id, wallet_id, payment_id, type, amount, currency, status, description, created_at, correlation_id
		FROM transactions WHERE wallet_id = $1 ORDER BY created_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query, walletID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var transactions []*wallet.Transaction
	for rows.Next() {
		txn := &wallet.Transaction{}
		err := rows.Scan(
			&txn.ID, &txn.WalletID, &txn.PaymentID, &txn.Type, &txn.Amount,
			&txn.Currency, &txn.Status, &txn.Description, &txn.CreatedAt, &txn.CorrelationID)
		if err != nil {
			return nil, err
		}
		transactions = append(transactions, txn)
	}

	return transactions, nil
}

// MetricsRepository implementación de repositorio de métricas
type MetricsRepository struct {
	db *DB
}

// NewMetricsRepository crea un nuevo repositorio de métricas
func NewMetricsRepository(db *DB) *MetricsRepository {
	return &MetricsRepository{db: db}
}

// SaveMetric guarda una métrica
func (r *MetricsRepository) SaveMetric(ctx context.Context, metricName string, value float64, labels map[string]string, timestamp time.Time) error {
	labelsJSON, _ := json.Marshal(labels)
	query := `
		INSERT INTO metrics (metric_name, value, labels, timestamp)
		VALUES ($1, $2, $3, $4)
	`
	_, err := r.db.ExecContext(ctx, query, metricName, value, labelsJSON, timestamp)
	return err
}

// GetMetrics obtiene métricas por nombre y rango de tiempo
func (r *MetricsRepository) GetMetrics(ctx context.Context, metricName string, from, to time.Time) ([]metrics.MetricPoint, error) {
	query := `
		SELECT timestamp, value, labels
		FROM metrics 
		WHERE metric_name = $1 AND timestamp BETWEEN $2 AND $3
		ORDER BY timestamp ASC
	`

	rows, err := r.db.QueryContext(ctx, query, metricName, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []metrics.MetricPoint
	for rows.Next() {
		var point metrics.MetricPoint
		var labelsJSON []byte

		err := rows.Scan(&point.Timestamp, &point.Value, &labelsJSON)
		if err != nil {
			return nil, err
		}

		json.Unmarshal(labelsJSON, &point.Labels)
		points = append(points, point)
	}

	return points, nil
}

// SaveAlert guarda una alerta
func (r *MetricsRepository) SaveAlert(ctx context.Context, alert *metrics.Alert) error {
	labelsJSON, _ := json.Marshal(alert.Labels)
	annotationsJSON, _ := json.Marshal(alert.Annotations)

	query := `
		INSERT INTO alerts (id, rule_id, rule_name, metric_name, value, threshold, condition, severity, message, labels, annotations, fired_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`
	_, err := r.db.ExecContext(ctx, query,
		alert.ID, alert.RuleID, alert.RuleName, alert.MetricName, alert.Value,
		alert.Threshold, alert.Condition, alert.Severity, alert.Message,
		labelsJSON, annotationsJSON, alert.FiredAt)

	return err
}

// GetActiveAlerts obtiene alertas activas
func (r *MetricsRepository) GetActiveAlerts(ctx context.Context) ([]*metrics.Alert, error) {
	query := `
		SELECT id, rule_id, rule_name, metric_name, value, threshold, condition, severity, message, labels, annotations, fired_at
		FROM alerts 
		WHERE fired_at > $1
		ORDER BY fired_at DESC
	`

	// Alertas de las últimas 24 horas
	since := time.Now().Add(-24 * time.Hour)

	rows, err := r.db.QueryContext(ctx, query, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alerts []*metrics.Alert
	for rows.Next() {
		alert := &metrics.Alert{}
		var labelsJSON, annotationsJSON []byte

		err := rows.Scan(
			&alert.ID, &alert.RuleID, &alert.RuleName, &alert.MetricName,
			&alert.Value, &alert.Threshold, &alert.Condition, &alert.Severity,
			&alert.Message, &labelsJSON, &annotationsJSON, &alert.FiredAt)
		if err != nil {
			return nil, err
		}

		json.Unmarshal(labelsJSON, &alert.Labels)
		json.Unmarshal(annotationsJSON, &alert.Annotations)
		alerts = append(alerts, alert)
	}

	return alerts, nil
}

// SagaRepository implementación de repositorio de sagas
type SagaRepository struct {
	db *DB
}

// NewSagaRepository crea un nuevo repositorio de sagas
func NewSagaRepository(db *DB) *SagaRepository {
	return &SagaRepository{db: db}
}

// SaveSaga guarda una saga
func (r *SagaRepository) SaveSaga(ctx context.Context, s *saga.Saga) error {
	stepsJSON, _ := json.Marshal(s.Steps)
	contextJSON, _ := json.Marshal(s.Context)

	query := `
		INSERT INTO sagas (id, type, state, steps, current_step, context, created_at, updated_at, completed_at, correlation_id, user_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (id) DO UPDATE SET
			state = EXCLUDED.state,
			steps = EXCLUDED.steps,
			current_step = EXCLUDED.current_step,
			context = EXCLUDED.context,
			updated_at = EXCLUDED.updated_at,
			completed_at = EXCLUDED.completed_at
	`
	_, err := r.db.ExecContext(ctx, query,
		s.ID, s.Type, s.State, stepsJSON, s.CurrentStep, contextJSON,
		s.CreatedAt, s.UpdatedAt, s.CompletedAt, s.CorrelationID, s.UserID)

	return err
}

// GetSaga obtiene una saga por ID
func (r *SagaRepository) GetSaga(ctx context.Context, id uuid.UUID) (*saga.Saga, error) {
	query := `
		SELECT id, type, state, steps, current_step, context, created_at, updated_at, completed_at, correlation_id, user_id
		FROM sagas WHERE id = $1
	`

	s := &saga.Saga{}
	var stepsJSON, contextJSON []byte

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&s.ID, &s.Type, &s.State, &stepsJSON, &s.CurrentStep, &contextJSON,
		&s.CreatedAt, &s.UpdatedAt, &s.CompletedAt, &s.CorrelationID, &s.UserID)
	if err != nil {
		return nil, err
	}

	json.Unmarshal(stepsJSON, &s.Steps)
	json.Unmarshal(contextJSON, &s.Context)

	return s, nil
}

// UpdateSagaState actualiza el estado de una saga
func (r *SagaRepository) UpdateSagaState(ctx context.Context, id uuid.UUID, state saga.SagaState) error {
	query := `UPDATE sagas SET state = $1, updated_at = $2 WHERE id = $3`
	_, err := r.db.ExecContext(ctx, query, state, time.Now(), id)
	return err
}

// UpdateStepStatus actualiza el estado de un paso de saga
func (r *SagaRepository) UpdateStepStatus(ctx context.Context, sagaID uuid.UUID, stepID uuid.UUID, status saga.StepStatus, output map[string]interface{}, errorMsg string) error {
	// Primero obtenemos la saga actual
	s, err := r.GetSaga(ctx, sagaID)
	if err != nil {
		return err
	}

	// Actualizamos el paso específico
	for _, step := range s.Steps {
		if step.ID == stepID {
			step.Status = status
			if output != nil {
				step.Output = output
			}
			if errorMsg != "" {
				step.Error = errorMsg
			}
			break
		}
	}

	// Guardamos la saga actualizada
	return r.SaveSaga(ctx, s)
}

// GetPendingSagas obtiene sagas pendientes
func (r *SagaRepository) GetPendingSagas(ctx context.Context) ([]*saga.Saga, error) {
	query := `
		SELECT id, type, state, steps, current_step, context, created_at, updated_at, completed_at, correlation_id, user_id
		FROM sagas 
		WHERE state IN ('started', 'in_progress', 'failed')
		ORDER BY created_at ASC
	`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sagas []*saga.Saga
	for rows.Next() {
		s := &saga.Saga{}
		var stepsJSON, contextJSON []byte

		err := rows.Scan(
			&s.ID, &s.Type, &s.State, &stepsJSON, &s.CurrentStep, &contextJSON,
			&s.CreatedAt, &s.UpdatedAt, &s.CompletedAt, &s.CorrelationID, &s.UserID)
		if err != nil {
			return nil, err
		}

		json.Unmarshal(stepsJSON, &s.Steps)
		json.Unmarshal(contextJSON, &s.Context)
		sagas = append(sagas, s)
	}

	return sagas, nil
}
