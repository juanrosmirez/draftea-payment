package test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"payment-system/internal/config"
	"payment-system/internal/services/payment"
	"payment-system/internal/services/wallet"
	"payment-system/pkg/database"
	"payment-system/pkg/logger"
	"payment-system/pkg/server"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type IntegrationTestSuite struct {
	suite.Suite
	router         *gin.Engine
	paymentService *payment.Service
	walletService  *wallet.Service
	db             *database.DB
	paymentRepo    *MockPaymentRepository
}

func (suite *IntegrationTestSuite) SetupSuite() {
	// Setup test configuration
	cfg := &config.Config{
		Database: config.DatabaseConfig{
			Host:     "localhost",
			Port:     5432,
			Database: "payment_system_test",
			Username: "postgres",
			Password: "postgres123",
			SSLMode:  "disable",
		},
		Logging: config.LoggingConfig{
			Level:  "debug",
			Format: "json",
		},
	}

	// Initialize logger
	logger := logger.New(cfg.Logging.Level, cfg.Logging.Format)

	// Initialize database (in real integration tests, use test database)
	// For this example, we'll use mocks or in-memory database
	// db, err := database.New(cfg.Database)
	// suite.Require().NoError(err)
	// suite.db = db

	// Initialize event publisher (mock for testing)
	eventPublisher := &MockEventPublisher{}

	// Initialize repositories (would use real DB in actual integration test)
	suite.paymentRepo = &MockPaymentRepository{}
	walletRepo := &MockWalletRepository{}

	// Initialize services
	suite.walletService = wallet.NewService(walletRepo, eventPublisher, logger)
	gatewayService := &MockGatewayService{}
	suite.paymentService = payment.NewService(suite.paymentRepo, suite.walletService, gatewayService, eventPublisher, logger)

	// Setup router
	gin.SetMode(gin.TestMode)
	suite.router = gin.New()
	
	// Setup routes
	api := suite.router.Group("/api/v1")
	api.POST("/payments", server.CreatePaymentHandler(suite.paymentService))
	api.GET("/payments/:id", server.GetPaymentHandler(suite.paymentService))
	api.GET("/wallets/:user_id/:currency", server.GetWalletHandler(suite.walletService))
}

func (suite *IntegrationTestSuite) TearDownSuite() {
	if suite.db != nil {
		suite.db.Close()
	}
}

func (suite *IntegrationTestSuite) TestCreatePayment_Success() {
	// Prepare request
	userID := uuid.New()
	paymentReq := payment.PaymentRequest{
		UserID:      userID,
		Amount:      10000,
		Currency:    "USD",
		ServiceID:   "netflix",
		Description: "Netflix subscription",
	}

	reqBody, _ := json.Marshal(paymentReq)
	req := httptest.NewRequest("POST", "/api/v1/payments", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")

	// Execute request
	w := httptest.NewRecorder()
	suite.router.ServeHTTP(w, req)

	// Assert response
	assert.Equal(suite.T(), http.StatusCreated, w.Code)

	var response payment.Payment
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), userID, response.UserID)
	assert.Equal(suite.T(), int64(10000), response.Amount)
	assert.Equal(suite.T(), "USD", response.Currency)
}

func (suite *IntegrationTestSuite) TestCreatePayment_InvalidRequest() {
	// Prepare invalid request (missing required fields)
	paymentReq := payment.PaymentRequest{
		Amount:   -1000, // Invalid negative amount
		Currency: "",    // Missing currency
	}

	reqBody, _ := json.Marshal(paymentReq)
	req := httptest.NewRequest("POST", "/api/v1/payments", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")

	// Execute request
	w := httptest.NewRecorder()
	suite.router.ServeHTTP(w, req)

	// Assert response
	assert.Equal(suite.T(), http.StatusBadRequest, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(suite.T(), err)
	assert.Contains(suite.T(), response, "error")
}

func (suite *IntegrationTestSuite) TestGetPayment_Success() {
	// First create a payment
	userID := uuid.New()
	paymentID := uuid.New()
	
	// Create a payment and save it to the mock repository
	testPayment := &payment.Payment{
		ID:       paymentID,
		UserID:   userID,
		Amount:   10000,
		Currency: "USD",
		Status:   payment.StatusCompleted,
	}
	
	// Save the payment to the mock repository
	suite.paymentRepo.Save(context.Background(), testPayment)

	// Execute request
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/payments/%s", paymentID.String()), nil)
	w := httptest.NewRecorder()
	suite.router.ServeHTTP(w, req)

	// Assert response
	assert.Equal(suite.T(), http.StatusOK, w.Code)

	var response payment.Payment
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), paymentID, response.ID)
}

func (suite *IntegrationTestSuite) TestGetPayment_NotFound() {
	nonExistentID := uuid.New()

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/payments/%s", nonExistentID.String()), nil)
	w := httptest.NewRecorder()
	suite.router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusNotFound, w.Code)
}

func (suite *IntegrationTestSuite) TestGetWallet_Success() {
	userID := uuid.New()
	currency := "USD"

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/wallets/%s/%s", userID.String(), currency), nil)
	w := httptest.NewRecorder()
	suite.router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusOK, w.Code)

	var response wallet.Wallet
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), userID, response.UserID)
	assert.Equal(suite.T(), currency, response.Currency)
}

func (suite *IntegrationTestSuite) TestPaymentFlow_EndToEnd() {
	// This test simulates a complete payment flow
	userID := uuid.New()

	// Step 1: Create payment
	paymentReq := payment.PaymentRequest{
		UserID:      userID,
		Amount:      5000,
		Currency:    "USD",
		ServiceID:   "spotify",
		Description: "Spotify Premium",
	}

	reqBody, _ := json.Marshal(paymentReq)
	req := httptest.NewRequest("POST", "/api/v1/payments", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	suite.router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusCreated, w.Code)

	var createdPayment payment.Payment
	err := json.Unmarshal(w.Body.Bytes(), &createdPayment)
	assert.NoError(suite.T(), err)

	// Step 2: Verify payment was created
	req = httptest.NewRequest("GET", fmt.Sprintf("/api/v1/payments/%s", createdPayment.ID.String()), nil)
	w = httptest.NewRecorder()
	suite.router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusOK, w.Code)

	// Step 3: Check wallet balance (would be updated in real scenario)
	req = httptest.NewRequest("GET", fmt.Sprintf("/api/v1/wallets/%s/USD", userID.String()), nil)
	w = httptest.NewRecorder()
	suite.router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusOK, w.Code)
}

// Mock implementations for integration tests
type MockEventPublisher struct{}

func (m *MockEventPublisher) Publish(ctx context.Context, event interface{}) error {
	return nil
}

func (m *MockEventPublisher) PublishBatch(ctx context.Context, events []interface{}) error {
	return nil
}

func (m *MockEventPublisher) Close() error {
	return nil
}

type MockPaymentRepository struct {
	payments map[uuid.UUID]*payment.Payment
	mutex    sync.RWMutex
}

func (m *MockPaymentRepository) Save(ctx context.Context, p *payment.Payment) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	
	if m.payments == nil {
		m.payments = make(map[uuid.UUID]*payment.Payment)
	}
	m.payments[p.ID] = p
	return nil
}

func (m *MockPaymentRepository) GetByID(ctx context.Context, id uuid.UUID) (*payment.Payment, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	
	if m.payments == nil {
		return nil, fmt.Errorf("payment not found")
	}
	
	if p, exists := m.payments[id]; exists {
		return p, nil
	}
	
	// Return error for non-existent payment (this will cause 404 in handler)
	return nil, fmt.Errorf("payment not found")
}

func (m *MockPaymentRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status payment.PaymentStatus) error {
	return nil
}

type MockWalletRepository struct{}

func (m *MockWalletRepository) GetWalletByUserID(ctx context.Context, userID uuid.UUID, currency string) (*wallet.Wallet, error) {
	return &wallet.Wallet{
		ID:       uuid.New(),
		UserID:   userID,
		Balance:  100000,
		Currency: currency,
		Version:  1,
	}, nil
}

func (m *MockWalletRepository) UpdateBalance(ctx context.Context, walletID uuid.UUID, newBalance int64, version int) error {
	return nil
}

func (m *MockWalletRepository) CreateTransaction(ctx context.Context, txn *wallet.Transaction) error {
	return nil
}

func (m *MockWalletRepository) GetTransactionsByWallet(ctx context.Context, walletID uuid.UUID) ([]*wallet.Transaction, error) {
	return []*wallet.Transaction{}, nil
}

type MockGatewayService struct{}

func (m *MockGatewayService) ProcessPayment(ctx context.Context, p *payment.Payment) (string, error) {
	return "gw_test_12345", nil
}

// Run the integration test suite
func TestIntegrationSuite(t *testing.T) {
	suite.Run(t, new(IntegrationTestSuite))
}

// Load testing
func TestPaymentLoad(t *testing.T) {
	// Setup similar to integration test
	gin.SetMode(gin.TestMode)
	router := gin.New()
	
	// Mock services
	eventPublisher := &MockEventPublisher{}
	paymentRepo := &MockPaymentRepository{}
	walletRepo := &MockWalletRepository{}
	gatewayService := &MockGatewayService{}
	
	logger := logger.New("error", "json")
	walletService := wallet.NewService(walletRepo, eventPublisher, logger)
	paymentService := payment.NewService(paymentRepo, walletService, gatewayService, eventPublisher, logger)

	api := router.Group("/api/v1")
	api.POST("/payments", server.CreatePaymentHandler(paymentService))

	// Prepare test data
	userID := uuid.New()
	paymentReq := payment.PaymentRequest{
		UserID:      userID,
		Amount:      1000,
		Currency:    "USD",
		ServiceID:   "test",
		Description: "Load test",
	}
	reqBody, _ := json.Marshal(paymentReq)

	// Run concurrent requests
	concurrency := 10
	requests := 100
	
	start := time.Now()
	
	for i := 0; i < concurrency; i++ {
		go func() {
			for j := 0; j < requests/concurrency; j++ {
				req := httptest.NewRequest("POST", "/api/v1/payments", bytes.NewBuffer(reqBody))
				req.Header.Set("Content-Type", "application/json")
				
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)
				
				if w.Code != http.StatusCreated {
					t.Errorf("Expected status 201, got %d", w.Code)
				}
			}
		}()
	}
	
	duration := time.Since(start)
	t.Logf("Processed %d requests in %v (%.2f req/sec)", requests, duration, float64(requests)/duration.Seconds())
}
