-- Crear extensiones necesarias
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Tabla de pagos
CREATE TABLE IF NOT EXISTS payments (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL,
    amount BIGINT NOT NULL CHECK (amount > 0),
    currency VARCHAR(3) NOT NULL,
    service_id VARCHAR(255) NOT NULL,
    description TEXT,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    gateway_txn_id VARCHAR(255),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    correlation_id UUID NOT NULL
);

-- Índices para pagos
CREATE INDEX IF NOT EXISTS idx_payments_user_id ON payments(user_id);
CREATE INDEX IF NOT EXISTS idx_payments_status ON payments(status);
CREATE INDEX IF NOT EXISTS idx_payments_created_at ON payments(created_at);
CREATE INDEX IF NOT EXISTS idx_payments_correlation_id ON payments(correlation_id);

-- Tabla de billeteras
CREATE TABLE IF NOT EXISTS wallets (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL,
    balance BIGINT NOT NULL DEFAULT 0 CHECK (balance >= 0),
    currency VARCHAR(3) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    version INTEGER NOT NULL DEFAULT 1,
    UNIQUE(user_id, currency)
);

-- Índices para billeteras
CREATE INDEX IF NOT EXISTS idx_wallets_user_id ON wallets(user_id);
CREATE INDEX IF NOT EXISTS idx_wallets_currency ON wallets(currency);

-- Tabla de transacciones
CREATE TABLE IF NOT EXISTS transactions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    wallet_id UUID NOT NULL REFERENCES wallets(id),
    payment_id UUID REFERENCES payments(id),
    type VARCHAR(10) NOT NULL CHECK (type IN ('debit', 'credit')),
    amount BIGINT NOT NULL CHECK (amount > 0),
    currency VARCHAR(3) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    description TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    correlation_id UUID NOT NULL
);

-- Índices para transacciones
CREATE INDEX IF NOT EXISTS idx_transactions_wallet_id ON transactions(wallet_id);
CREATE INDEX IF NOT EXISTS idx_transactions_payment_id ON transactions(payment_id);
CREATE INDEX IF NOT EXISTS idx_transactions_created_at ON transactions(created_at);
CREATE INDEX IF NOT EXISTS idx_transactions_correlation_id ON transactions(correlation_id);

-- Tabla de métricas
CREATE TABLE IF NOT EXISTS metrics (
    id SERIAL PRIMARY KEY,
    metric_name VARCHAR(255) NOT NULL,
    value DOUBLE PRECISION NOT NULL,
    labels JSONB,
    timestamp TIMESTAMP WITH TIME ZONE NOT NULL
);

-- Índices para métricas
CREATE INDEX IF NOT EXISTS idx_metrics_name_timestamp ON metrics(metric_name, timestamp);
CREATE INDEX IF NOT EXISTS idx_metrics_timestamp ON metrics(timestamp);
CREATE INDEX IF NOT EXISTS idx_metrics_labels ON metrics USING GIN(labels);

-- Tabla de alertas
CREATE TABLE IF NOT EXISTS alerts (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    rule_id UUID NOT NULL,
    rule_name VARCHAR(255) NOT NULL,
    metric_name VARCHAR(255) NOT NULL,
    value DOUBLE PRECISION NOT NULL,
    threshold DOUBLE PRECISION NOT NULL,
    condition VARCHAR(10) NOT NULL,
    severity VARCHAR(20) NOT NULL,
    message TEXT NOT NULL,
    labels JSONB,
    annotations JSONB,
    fired_at TIMESTAMP WITH TIME ZONE NOT NULL
);

-- Índices para alertas
CREATE INDEX IF NOT EXISTS idx_alerts_rule_id ON alerts(rule_id);
CREATE INDEX IF NOT EXISTS idx_alerts_fired_at ON alerts(fired_at);
CREATE INDEX IF NOT EXISTS idx_alerts_severity ON alerts(severity);

-- Tabla de sagas
CREATE TABLE IF NOT EXISTS sagas (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    type VARCHAR(50) NOT NULL,
    state VARCHAR(50) NOT NULL,
    steps JSONB NOT NULL,
    current_step INTEGER NOT NULL DEFAULT 0,
    context JSONB,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    completed_at TIMESTAMP WITH TIME ZONE,
    correlation_id UUID NOT NULL,
    user_id UUID NOT NULL
);

-- Índices para sagas
CREATE INDEX IF NOT EXISTS idx_sagas_state ON sagas(state);
CREATE INDEX IF NOT EXISTS idx_sagas_user_id ON sagas(user_id);
CREATE INDEX IF NOT EXISTS idx_sagas_created_at ON sagas(created_at);
CREATE INDEX IF NOT EXISTS idx_sagas_correlation_id ON sagas(correlation_id);

-- Función para actualizar updated_at automáticamente
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Triggers para actualizar updated_at
CREATE TRIGGER update_payments_updated_at BEFORE UPDATE ON payments
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_wallets_updated_at BEFORE UPDATE ON wallets
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_sagas_updated_at BEFORE UPDATE ON sagas
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Insertar datos de ejemplo para testing
INSERT INTO wallets (user_id, balance, currency) VALUES 
    ('550e8400-e29b-41d4-a716-446655440001', 100000, 'USD'),
    ('550e8400-e29b-41d4-a716-446655440002', 50000, 'USD'),
    ('550e8400-e29b-41d4-a716-446655440001', 200000, 'EUR')
ON CONFLICT (user_id, currency) DO NOTHING;
