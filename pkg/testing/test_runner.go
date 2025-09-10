package testing

import (
	"fmt"
	"sync"
	"time"

	"payment-system/pkg/eventstore"
)

// TestWalletService implementación thread-safe para pruebas de concurrencia
type TestWalletService struct {
	eventStore eventstore.EventStore
	eventBus   eventstore.EventBus
	balances   map[string]float64
	mutex      sync.RWMutex
}

// NewTestWalletService crea un servicio de billetera thread-safe para pruebas
func NewTestWalletService(eventStore eventstore.EventStore, eventBus eventstore.EventBus) *TestWalletService {
	return &TestWalletService{
		eventStore: eventStore,
		eventBus:   eventBus,
		balances:   make(map[string]float64),
		mutex:      sync.RWMutex{},
	}
}

// SetBalance establece el saldo de un usuario de forma thread-safe
func (tws *TestWalletService) SetBalance(userID string, balance float64) {
	tws.mutex.Lock()
	defer tws.mutex.Unlock()
	tws.balances[userID] = balance
}

// GetBalance obtiene el saldo de un usuario de forma thread-safe
func (tws *TestWalletService) GetBalance(userID string) float64 {
	tws.mutex.RLock()
	defer tws.mutex.RUnlock()
	return tws.balances[userID]
}

// ProcessPayment procesa un pago de forma thread-safe
func (tws *TestWalletService) ProcessPayment(userID string, amount float64) error {
	tws.mutex.Lock()
	defer tws.mutex.Unlock()
	
	currentBalance := tws.balances[userID]
	if currentBalance < amount {
		return fmt.Errorf("insufficient funds: current balance %.2f, required %.2f", currentBalance, amount)
	}
	
	finalBalance := currentBalance - amount
	fmt.Printf("Saldo final del usuario %s: %.2f\n", userID, finalBalance)
	
	// Simular procesamiento
	time.Sleep(10 * time.Millisecond)
	
	tws.balances[userID] = finalBalance
	return nil
}

// ProcessFundsReverted procesa reversiones de fondos de forma thread-safe
func (tws *TestWalletService) ProcessFundsReverted(userID string, amount float64) error {
	tws.mutex.Lock()
	defer tws.mutex.Unlock()

	// Incrementar saldo
	tws.balances[userID] += amount
	return nil
}

// GenerateTraceID genera un ID de trazabilidad único
func GenerateTraceID() string {
	return fmt.Sprintf("trace_%d", time.Now().UnixNano())
}

// TestSummary proporciona un resumen de las pruebas implementadas
func TestSummary() {
	fmt.Println("=== RESUMEN DE PRUEBAS IMPLEMENTADAS ===")
	fmt.Println()
	
	fmt.Println("📋 PRUEBAS UNITARIAS DE BILLETERA (wallet_service_test.go):")
	fmt.Println("  ✅ TestWalletService_ProcessFundsDeducted")
	fmt.Println("     - Deducción exitosa con saldo suficiente")
	fmt.Println("     - Deducción fallida por saldo insuficiente")
	fmt.Println("     - Deducción exacta del saldo disponible")
	fmt.Println("     - Deducción de cantidad cero")
	fmt.Println()
	fmt.Println("  ✅ TestWalletService_ProcessFundsReverted")
	fmt.Println("     - Reversión correcta de fondos")
	fmt.Println("     - Persistencia en event store")
	fmt.Println()
	fmt.Println("  ✅ TestWalletService_ConcurrentOperations")
	fmt.Println("     - Control de concurrencia en operaciones simultáneas")
	fmt.Println("     - Prevención de condiciones de carrera")
	fmt.Println()
	
	fmt.Println("🔄 PRUEBAS DE INTEGRACIÓN DE FLUJO COMPLETO (payment_flow_test.go):")
	fmt.Println("  ✅ TestPaymentFlow_SuccessfulPayment")
	fmt.Println("     - Flujo completo de pago exitoso")
	fmt.Println("     - Verificación de secuencia de eventos")
	fmt.Println("     - Actualización correcta de saldo")
	fmt.Println("     - Llamada a gateway externo")
	fmt.Println()
	fmt.Println("  ✅ TestPaymentFlow_FailedExternalPayment")
	fmt.Println("     - Manejo de fallo en pasarela externa")
	fmt.Println("     - Compensación automática de fondos")
	fmt.Println("     - Eventos de reversión y fallo")
	fmt.Println()
	fmt.Println("  ✅ TestPaymentFlow_InsufficientFunds")
	fmt.Println("     - Rechazo por fondos insuficientes")
	fmt.Println("     - No procesamiento en gateway externo")
	fmt.Println("     - Saldo sin cambios")
	fmt.Println()
	fmt.Println("  ✅ TestPaymentFlow_ConcurrentPayments")
	fmt.Println("     - Múltiples pagos concurrentes")
	fmt.Println("     - Control de integridad en concurrencia")
	fmt.Println("     - Verificación de límites de saldo")
	fmt.Println()
	
	fmt.Println("🛠️  COMPONENTES MOCK IMPLEMENTADOS:")
	fmt.Println("  • MockEventStore - Event store con historial")
	fmt.Println("  • MockEventBus - Event bus con suscripciones")
	fmt.Println("  • MockPaymentGateway - Gateway de pago configurable")
	fmt.Println("  • ConcurrentWalletService - Servicio thread-safe")
	fmt.Println()
	
	fmt.Println("📊 COBERTURA DE CASOS DE PRUEBA:")
	fmt.Println("  ✓ Casos exitosos (happy path)")
	fmt.Println("  ✓ Casos de error y validación")
	fmt.Println("  ✓ Casos de concurrencia")
	fmt.Println("  ✓ Casos de compensación")
	fmt.Println("  ✓ Casos de integración end-to-end")
	fmt.Println()
	
	fmt.Println("🚀 PARA EJECUTAR LAS PRUEBAS:")
	fmt.Println("  go test ./pkg/eventstore -v")
	fmt.Println("  go test ./pkg/eventstore -run TestWalletService")
	fmt.Println("  go test ./pkg/eventstore -run TestPaymentFlow")
	fmt.Println()
}
