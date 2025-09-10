@echo off
echo ========================================
echo        CREATE PAYMENT TEST
echo ========================================
echo Testing: POST http://localhost:8081/api/v1/payments
echo User: 550e8400-e29b-41d4-a716-446655440001
echo Amount: $100.00 USD
echo Service: Netflix Subscription
echo.

curl -X POST http://localhost:8081/api/v1/payments ^
  -H "Content-Type: application/json" ^
  -d "{\"user_id\":\"550e8400-e29b-41d4-a716-446655440001\",\"amount\":10000,\"currency\":\"USD\",\"service_id\":\"netflix_subscription\",\"description\":\"Netflix Monthly Subscription\"}"

echo.
echo ========================================
echo IMPORTANTE: Guarda el 'id' del pago para usar en get-payment.bat
echo Test completed. Press any key to exit.
pause > nul
