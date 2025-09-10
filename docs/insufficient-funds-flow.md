# Escenario 2 - Saldo Insuficiente: Flujo de Pago Rechazado

## Diagrama de Secuencia - Saldo Insuficiente

```mermaid
sequenceDiagram
    participant U as Usuario/Cliente
    participant API as API Gateway
    participant PS as Payment Service
    participant WS as Wallet Service
    participant K as Kafka Event Bus
    participant SO as Saga Orchestrator
    participant MS as Metrics Service
    participant NS as Notification Service
    participant DB as PostgreSQL
    participant R as Redis Cache

    Note over U,R: ❌ ESCENARIO: SALDO INSUFICIENTE

    %% 1. Iniciación del Pago
    U->>+API: POST /api/v1/payments<br/>{user_id, amount: 1000, currency: "USD"}
    API->>+PS: CreatePayment Request
    
    %% 2. Validación y Creación
    PS->>DB: Validar datos del pago
    PS->>DB: Crear registro de pago (status: INITIATED)
    PS->>+K: Publish PaymentInitiated Event
    Note right of K: PaymentInitiated<br/>{payment_id, user_id, amount: 1000}
    
    %% 3. Saga Orchestrator recibe evento
    K->>+SO: PaymentInitiated Event
    SO->>DB: Crear nueva saga de pago
    SO->>+K: Publish SagaStarted Event
    Note right of K: SagaStarted<br/>{saga_id, payment_id, steps}
    
    %% 4. Metrics Service registra inicio
    K->>+MS: PaymentInitiated Event
    MS->>DB: Registrar métrica de pago iniciado
    MS->>R: Incrementar contador de pagos iniciados
    
    %% 5. Verificación de Saldo - FALLA
    SO->>+WS: CheckWalletBalance Command<br/>{wallet_id, requested_amount: 1000}
    WS->>DB: SELECT balance FROM wallets WHERE id = $1
    Note right of DB: balance = 250 USD<br/>(Insuficiente para 1000 USD)
    WS->>R: Cache miss - consultar DB
    
    %% 6. Detección de Saldo Insuficiente
    WS->>+K: Publish WalletBalanceChecked Event
    Note right of K: WalletBalanceChecked<br/>{wallet_id, sufficient_funds: false,<br/>available_balance: 250, requested: 1000}
    
    %% 7. Saga detecta fallo y no procede con deducción
    K->>SO: WalletBalanceChecked Event (sufficient_funds: false)
    SO->>DB: UPDATE saga SET status = 'FAILED', failed_step = 'WALLET_VALIDATION'
    
    %% 8. Payment Service marca pago como fallido
    SO->>+PS: MarkPaymentFailed Command<br/>{reason: "INSUFFICIENT_FUNDS"}
    PS->>DB: UPDATE payments SET status = 'FAILED', error_code = 'INSUFFICIENT_FUNDS'
    PS->>+K: Publish PaymentFailed Event
    Note right of K: PaymentFailed<br/>{payment_id, error_code: "INSUFFICIENT_FUNDS",<br/>available_balance: 250, required: 1000}
    
    %% 9. Saga finaliza con fallo
    K->>SO: PaymentFailed Event
    SO->>+K: Publish SagaFailed Event
    Note right of K: SagaFailed<br/>{saga_id, failed_step: "WALLET_VALIDATION",<br/>failure_reason: "INSUFFICIENT_FUNDS"}
    
    %% 10. Actualización de Métricas de Fallo
    K->>MS: PaymentFailed Event
    MS->>DB: Registrar métrica de pago fallido
    MS->>R: Incrementar contador de fallos por fondos insuficientes
    K->>MS: SagaFailed Event
    MS->>R: Registrar fallo de saga por validación
    
    %% 11. Notificación al Usuario
    K->>+NS: PaymentFailed Event
    NS->>U: Enviar notificación push/email<br/>"Pago rechazado: Saldo insuficiente<br/>Disponible: $250, Requerido: $1000"
    
    %% 12. Respuesta de Error al Cliente
    PS-->>-API: PaymentErrorResponse<br/>{error: "INSUFFICIENT_FUNDS", available: 250}
    API-->>-U: 400 Bad Request<br/>{error: "Saldo insuficiente", available_balance: 250}
    
    Note over U,R: ❌ PAGO RECHAZADO - NO SE CONTACTA PASARELA

    %% Activaciones
    deactivate K
    deactivate SO
    deactivate MS
    deactivate NS
    deactivate WS
    deactivate PS
```

## Eventos Clave del Flujo de Rechazo

### 1. PaymentInitiated
```yaml
Productor: Payment Service
Consumidores: [Saga Orchestrator, Metrics Service]
Propósito: Inicia el proceso de pago (igual que ruta feliz)
Schema:
  payment_id: UUID
  user_id: UUID
  amount: decimal
  currency: string
  initiated_at: timestamp
```

### 2. WalletBalanceChecked (Fondos Insuficientes)
```yaml
Productor: Wallet Service
Consumidores: [Saga Orchestrator]
Propósito: Reporta saldo insuficiente para el pago
Schema:
  wallet_id: UUID
  user_id: UUID
  requested_amount: decimal
  available_balance: decimal
  sufficient_funds: false  # ❌ CLAVE: false
  checked_at: timestamp
  deficit_amount: decimal  # requested - available
```

### 3. PaymentFailed
```yaml
Productor: Payment Service
Consumidores: [Saga Orchestrator, Metrics Service, Notification Service, Audit Service]
Propósito: Marca el pago como fallido por fondos insuficientes
Schema:
  payment_id: UUID
  user_id: UUID
  error_code: "INSUFFICIENT_FUNDS"
  error_message: "Saldo insuficiente para procesar el pago"
  available_balance: decimal
  required_amount: decimal
  deficit_amount: decimal
  failed_at: timestamp
  failure_stage: "WALLET_VALIDATION"
```

### 4. SagaFailed
```yaml
Productor: Saga Orchestrator
Consumidores: [Metrics Service, Audit Service]
Propósito: Registra fallo de saga sin necesidad de compensación
Schema:
  saga_id: UUID
  payment_id: UUID
  failed_step: "WALLET_VALIDATION"
  failure_reason: "INSUFFICIENT_FUNDS"
  total_duration_ms: integer
  completed_steps: 1  # Solo validación inicial
  failed_at: timestamp
```

## Diferencias Clave vs Ruta Feliz

### ❌ Lo que NO ocurre en este flujo:
1. **No hay deducción de billetera** - Se detiene en la verificación
2. **No se contacta la pasarela externa** - Ahorro de costos y latencia
3. **No hay eventos de compensación** - No hay nada que revertir
4. **No hay transacciones de dinero** - Solo consultas de saldo

### ✅ Lo que SÍ ocurre:
1. **Validación temprana** - Fallo rápido en verificación de saldo
2. **Métricas de fallo** - Registro para análisis de negocio
3. **Notificación clara** - Usuario informado del motivo específico
4. **Saga terminada limpiamente** - Sin necesidad de compensación

## Estados y Transiciones

### Estados del Pago
```
INITIATED → WALLET_VALIDATION_FAILED → FAILED
```

### Estados de la Saga
```
STARTED → WALLET_VALIDATION → FAILED (sin compensación)
```

## Consultas de Base de Datos

### 1. Verificación de Saldo
```sql
SELECT 
    id,
    balance,
    currency,
    status
FROM wallets 
WHERE user_id = $1 AND currency = $2 AND status = 'ACTIVE';
```

### 2. Marcado de Pago como Fallido
```sql
UPDATE payments 
SET 
    status = 'FAILED',
    error_code = 'INSUFFICIENT_FUNDS',
    error_message = 'Saldo insuficiente para procesar el pago',
    failed_at = NOW()
WHERE id = $1;
```

### 3. Registro de Saga Fallida
```sql
UPDATE sagas 
SET 
    status = 'FAILED',
    failed_step = 'WALLET_VALIDATION',
    failure_reason = 'INSUFFICIENT_FUNDS',
    completed_at = NOW()
WHERE id = $1;
```

## Métricas Específicas de Rechazo

### Métricas de Negocio
- `payments_failed_insufficient_funds_total`: Contador de rechazos por fondos
- `payment_rejection_rate`: Tasa de rechazo por saldo insuficiente
- `average_deficit_amount`: Promedio de déficit en rechazos
- `wallet_balance_distribution`: Distribución de saldos de usuarios

### Métricas Técnicas
- `saga_failures_by_step`: Fallos de saga por etapa
- `wallet_validation_duration`: Tiempo de validación de saldo
- `early_rejection_rate`: Tasa de rechazo temprano (antes de pasarela)

## Respuesta HTTP de Error

### Estructura de Respuesta
```json
{
  "error": {
    "code": "INSUFFICIENT_FUNDS",
    "message": "Saldo insuficiente para procesar el pago",
    "details": {
      "requested_amount": 1000.00,
      "available_balance": 250.00,
      "deficit_amount": 750.00,
      "currency": "USD"
    }
  },
  "payment_id": "550e8400-e29b-41d4-a716-446655440000",
  "timestamp": "2024-01-15T10:30:00Z"
}
```

### Código de Estado HTTP
```
400 Bad Request - Cliente envió solicitud inválida (saldo insuficiente)
```

## Notificación al Usuario

### Mensaje de Notificación
```yaml
Tipo: Push Notification + Email
Título: "Pago Rechazado"
Mensaje: "Tu pago de $1,000 USD no pudo procesarse debido a saldo insuficiente. 
         Saldo disponible: $250 USD. 
         Por favor, recarga tu billetera o reduce el monto."
Acciones:
  - "Recargar Billetera"
  - "Ver Saldo Actual"
  - "Intentar Nuevo Pago"
```

## Consideraciones de Performance

### Optimizaciones
1. **Fallo rápido**: ~50ms total (vs 300-600ms de ruta feliz)
2. **Sin llamadas externas**: No hay latencia de pasarela
3. **Cache de saldos**: Verificación optimizada con Redis
4. **Validación temprana**: Ahorra recursos del sistema

### Beneficios del Rechazo Temprano
- **Menor latencia** para el usuario
- **Menor costo** (no se cobra por llamada a pasarela)
- **Mejor UX** (respuesta inmediata y clara)
- **Menos carga** en servicios downstream
