@echo off
echo ========================================
echo      COMPLETE PAYMENT FLOW TEST
echo ========================================
echo This script demonstrates a complete payment workflow:
echo 1. Check system health
echo 2. Create a payment
echo 3. Retrieve the payment
echo 4. Check wallet balance
echo.

echo Step 1: Health Check
echo ========================================
curl -s http://localhost:8081/health
echo.

echo Step 2: Creating Payment...
echo ========================================
echo User: 550e8400-e29b-41d4-a716-446655440001
echo Amount: $75.50 USD
echo Service: Spotify Premium
echo.

for /f "tokens=*" %%i in ('curl -s -X POST http://localhost:8081/api/v1/payments -H "Content-Type: application/json" -d "{\"user_id\":\"550e8400-e29b-41d4-a716-446655440001\",\"amount\":7550,\"currency\":\"USD\",\"service_id\":\"spotify_premium\",\"description\":\"Spotify Premium Monthly\"}"') do set PAYMENT_ID=%%i

echo Payment created with ID: %PAYMENT_ID%
echo.

echo Step 3: Retrieving Payment...
echo ========================================
curl -s http://localhost:8081/api/v1/payments/%PAYMENT_ID%
echo.

echo Step 4: Checking Wallet Balance...
echo ========================================
curl -s http://localhost:8081/api/v1/wallets/550e8400-e29b-41d4-a716-446655440001/USD
echo.

echo ========================================
echo COMPLETE FLOW FINISHED
echo Payment ID: %PAYMENT_ID%
echo ========================================
pause
