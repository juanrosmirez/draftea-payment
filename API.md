# API Documentation - Sistema de Procesamiento de Pagos

## Descripción General

Este documento describe la API REST del sistema de procesamiento de pagos orientado a eventos. La API proporciona endpoints para gestionar pagos, billeteras, métricas y sagas.

## Base URL

```
http://localhost:8080/api/v1
```

## Autenticación

Actualmente la API no requiere autenticación. En producción se recomienda implementar JWT o API Keys.

## Endpoints

### Health Checks

#### GET /health
Verifica el estado de salud del servicio.

**Respuesta:**
```json
{
  "status": "healthy",
  "timestamp": "2024-01-15T10:30:00Z",
  "service": "payment-system",
  "version": "1.0.0"
}
```

#### GET /ready
Verifica si el servicio está listo para recibir tráfico.

**Respuesta:**
```json
{
  "status": "ready",
  "checks": {
    "database": "ok",
    "redis": "ok",
    "kafka": "ok"
  }
}
```

### Pagos

#### POST /api/v1/payments
Crea un nuevo pago.

**Request Body:**
```json
{
  "user_id": "550e8400-e29b-41d4-a716-446655440001",
  "amount": 10000,
  "currency": "USD",
  "service_id": "netflix_subscription",
  "description": "Netflix Monthly Subscription"
}
```

**Respuesta Exitosa (201):**
```json
{
  "id": "123e4567-e89b-12d3-a456-426614174000",
  "user_id": "550e8400-e29b-41d4-a716-446655440001",
  "amount": 10000,
  "currency": "USD",
  "service_id": "netflix_subscription",
  "description": "Netflix Monthly Subscription",
  "status": "processing",
  "gateway_txn_id": "",
  "created_at": "2024-01-15T10:30:00Z",
  "updated_at": "2024-01-15T10:30:00Z",
  "correlation_id": "456e7890-e12b-34d5-a678-901234567890"
}
```

**Errores:**
- `400 Bad Request`: Datos inválidos
- `500 Internal Server Error`: Error interno del servidor

#### GET /api/v1/payments/{id}
Obtiene un pago específico por ID.

**Parámetros:**
- `id` (UUID): ID del pago

**Respuesta Exitosa (200):**
```json
{
  "id": "123e4567-e89b-12d3-a456-426614174000",
  "user_id": "550e8400-e29b-41d4-a716-446655440001",
  "amount": 10000,
  "currency": "USD",
  "service_id": "netflix_subscription",
  "description": "Netflix Monthly Subscription",
  "status": "completed",
  "gateway_txn_id": "gw_stripe_1234567890",
  "created_at": "2024-01-15T10:30:00Z",
  "updated_at": "2024-01-15T10:32:15Z",
  "correlation_id": "456e7890-e12b-34d5-a678-901234567890"
}
```

**Errores:**
- `400 Bad Request`: ID inválido
- `404 Not Found`: Pago no encontrado

### Billeteras

#### GET /api/v1/wallets/{user_id}/{currency}
Obtiene la billetera de un usuario para una moneda específica.

**Parámetros:**
- `user_id` (UUID): ID del usuario
- `currency` (string): Código de moneda (USD, EUR, etc.)

**Respuesta Exitosa (200):**
```json
{
  "id": "789e0123-e45b-67c8-a901-234567890123",
  "user_id": "550e8400-e29b-41d4-a716-446655440001",
  "balance": 95000,
  "currency": "USD",
  "created_at": "2024-01-01T00:00:00Z",
  "updated_at": "2024-01-15T10:32:15Z",
  "version": 15
}
```

#### GET /api/v1/wallets/{user_id}/{currency}/transactions
Obtiene el historial de transacciones de una billetera.

**Parámetros:**
- `user_id` (UUID): ID del usuario
- `currency` (string): Código de moneda

**Respuesta Exitosa (200):**
```json
{
  "transactions": [
    {
      "id": "abc12345-def6-7890-ghij-klmnopqrstuv",
      "wallet_id": "789e0123-e45b-67c8-a901-234567890123",
      "payment_id": "123e4567-e89b-12d3-a456-426614174000",
      "type": "debit",
      "amount": 10000,
      "currency": "USD",
      "status": "completed",
      "description": "Payment deduction for payment 123e4567-e89b-12d3-a456-426614174000",
      "created_at": "2024-01-15T10:30:00Z",
      "correlation_id": "456e7890-e12b-34d5-a678-901234567890"
    }
  ],
  "count": 1
}
```

### Métricas

#### GET /api/v1/metrics
Obtiene métricas históricas.

**Query Parameters:**
- `metric` (string, requerido): Nombre de la métrica
- `from` (string, opcional): Timestamp de inicio (RFC3339)
- `to` (string, opcional): Timestamp de fin (RFC3339)

**Ejemplo:**
```
GET /api/v1/metrics?metric=payment_initiated&from=2024-01-15T00:00:00Z&to=2024-01-15T23:59:59Z
```

**Respuesta Exitosa (200):**
```json
{
  "metric_name": "payment_initiated",
  "from": "2024-01-15T00:00:00Z",
  "to": "2024-01-15T23:59:59Z",
  "points": [
    {
      "timestamp": "2024-01-15T10:30:00Z",
      "value": 1.0,
      "labels": {
        "payment_id": "123e4567-e89b-12d3-a456-426614174000",
        "user_id": "550e8400-e29b-41d4-a716-446655440001"
      }
    }
  ],
  "count": 1
}
```

#### GET /api/v1/metrics/alerts
Obtiene alertas activas.

**Respuesta Exitosa (200):**
```json
{
  "alerts": [
    {
      "id": "alert-123e4567-e89b-12d3-a456-426614174000",
      "rule_id": "rule-456e7890-e12b-34d5-a678-901234567890",
      "rule_name": "High Payment Failure Rate",
      "metric_name": "payment_failed",
      "value": 15.5,
      "threshold": 10.0,
      "condition": "gt",
      "severity": "warning",
      "message": "Alert: High Payment Failure Rate - payment_failed gt 10.00 (current: 15.50)",
      "labels": {
        "service": "payment-system"
      },
      "annotations": {},
      "fired_at": "2024-01-15T10:35:00Z"
    }
  ],
  "count": 1
}
```

#### POST /api/v1/metrics/alerts/rules
Crea una nueva regla de alerta.

**Request Body:**
```json
{
  "name": "High Payment Failure Rate",
  "metric_name": "payment_failed",
  "condition": "gt",
  "threshold": 10.0,
  "duration": "5m",
  "description": "Alert when payment failure rate exceeds 10%"
}
```

**Respuesta Exitosa (201):**
```json
{
  "id": "rule-456e7890-e12b-34d5-a678-901234567890",
  "name": "High Payment Failure Rate",
  "metric_name": "payment_failed",
  "condition": "gt",
  "threshold": 10.0,
  "duration": "5m0s",
  "enabled": true,
  "last_fired": "0001-01-01T00:00:00Z",
  "description": "Alert when payment failure rate exceeds 10%"
}
```

### Sagas

#### GET /api/v1/sagas/{id}
Obtiene información de una saga específica.

**Parámetros:**
- `id` (UUID): ID de la saga

**Respuesta Exitosa (200):**
```json
{
  "id": "saga-123e4567-e89b-12d3-a456-426614174000",
  "type": "payment",
  "state": "completed",
  "steps": [
    {
      "id": "step-1",
      "name": "validate_wallet_balance",
      "service_name": "wallet",
      "action": "validate_balance",
      "compensate_action": "",
      "status": "completed",
      "input": {
        "user_id": "550e8400-e29b-41d4-a716-446655440001",
        "amount": 10000,
        "currency": "USD"
      },
      "output": {
        "validated": true
      },
      "executed_at": "2024-01-15T10:30:01Z",
      "retry_count": 0,
      "max_retries": 3
    }
  ],
  "current_step": 4,
  "context": {
    "payment_id": "123e4567-e89b-12d3-a456-426614174000",
    "user_id": "550e8400-e29b-41d4-a716-446655440001",
    "amount": 10000,
    "currency": "USD"
  },
  "created_at": "2024-01-15T10:30:00Z",
  "updated_at": "2024-01-15T10:32:15Z",
  "completed_at": "2024-01-15T10:32:15Z",
  "correlation_id": "456e7890-e12b-34d5-a678-901234567890",
  "user_id": "550e8400-e29b-41d4-a716-446655440001"
}
```

## Estados de Pago

- `pending`: Pago creado pero no procesado
- `processing`: Pago en proceso de validación y ejecución
- `completed`: Pago completado exitosamente
- `failed`: Pago falló durante el procesamiento
- `cancelled`: Pago cancelado por el usuario

## Estados de Saga

- `started`: Saga iniciada
- `in_progress`: Saga en ejecución
- `completed`: Saga completada exitosamente
- `failed`: Saga falló durante la ejecución
- `compensated`: Saga falló y se ejecutó compensación

## Códigos de Error

- `400 Bad Request`: Solicitud malformada o datos inválidos
- `404 Not Found`: Recurso no encontrado
- `500 Internal Server Error`: Error interno del servidor

## Ejemplos de Uso

### Flujo Completo de Pago

1. **Crear un pago:**
```bash
curl -X POST http://localhost:8080/api/v1/payments \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "550e8400-e29b-41d4-a716-446655440001",
    "amount": 10000,
    "currency": "USD",
    "service_id": "netflix_subscription",
    "description": "Netflix Monthly Subscription"
  }'
```

2. **Verificar estado del pago:**
```bash
curl http://localhost:8080/api/v1/payments/123e4567-e89b-12d3-a456-426614174000
```

3. **Verificar saldo de billetera:**
```bash
curl http://localhost:8080/api/v1/wallets/550e8400-e29b-41d4-a716-446655440001/USD
```

4. **Ver historial de transacciones:**
```bash
curl http://localhost:8080/api/v1/wallets/550e8400-e29b-41d4-a716-446655440001/USD/transactions
```

### Monitoreo y Métricas

1. **Ver métricas de pagos:**
```bash
curl "http://localhost:8080/api/v1/metrics?metric=payment_initiated&from=2024-01-15T00:00:00Z"
```

2. **Ver alertas activas:**
```bash
curl http://localhost:8080/api/v1/metrics/alerts
```

3. **Crear regla de alerta:**
```bash
curl -X POST http://localhost:8080/api/v1/metrics/alerts/rules \
  -H "Content-Type: application/json" \
  -d '{
    "name": "High Payment Failure Rate",
    "metric_name": "payment_failed",
    "condition": "gt",
    "threshold": 10.0,
    "duration": "5m"
  }'
```
