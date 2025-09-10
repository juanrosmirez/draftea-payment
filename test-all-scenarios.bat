@echo off
echo ========================================
echo    PAYMENT SYSTEM - ALL TEST SCENARIOS
echo ========================================
echo.
echo Server should be running at: http://localhost:8080
echo.

:menu
echo ========================================
echo           MENU DE PRUEBAS COMPLETO
echo ========================================
echo.
echo === HEALTH CHECKS ===
echo 1. Health Check
echo 2. Readiness Check  
echo 3. Liveness Check

echo 4. Metrics (Prometheus)
echo.
echo === BASIC API TESTS ===
echo 5. Demo Payment Flow
echo 6. Get Wallet Balance
echo 7. Get Alerts
echo.
echo === HAPPY PATH FLOW ===
echo 8. Happy Path - Successful Payment ($100)
echo 9. Happy Path - Large Payment ($500)
echo 10. Happy Path - Small Payment ($10)
echo.
echo === INSUFFICIENT FUNDS FLOW ===
echo 11. Insufficient Funds - High Amount ($10000)
echo 12. Insufficient Funds - Medium Amount ($5000)
echo 13. Check Wallet After Failed Payment
echo.
echo === CONCURRENT PAYMENTS FLOW ===
echo 14. Concurrent Payments - Same User (Manual)
echo 15. Concurrent Payments - Setup Test Data
echo 16. Concurrent Payment A ($800)
echo 17. Concurrent Payment B ($600) - Should fail
echo.
echo === EDGE CASES ===
echo 18. Zero Amount Payment
echo 19. Negative Amount Payment
echo 20. Invalid Currency Payment
echo 21. Missing User ID Payment
echo.
echo === PAYMENT QUERIES ===
echo 22. Get Payment by ID (requires payment ID)
echo 23. Get All Payments for User
echo.
echo === WALLET OPERATIONS ===
echo 24. Get Wallet USD
echo 25. Get Wallet EUR
echo 26. Get Wallet Invalid Currency
echo.
echo === STRESS TESTS ===
echo 27. Multiple Small Payments (5x $20)
echo 28. Rapid Fire Payments (3 quick payments)
echo.
echo 29. Run All Happy Path Tests
echo 30. Run All Error Tests
echo 31. Exit
echo.
set /p choice="Selecciona una opcion (1-31): "

if "%choice%"=="1" goto health
if "%choice%"=="2" goto ready
if "%choice%"=="3" goto live
if "%choice%"=="4" goto metrics
if "%choice%"=="5" goto demo
if "%choice%"=="6" goto get_wallet_basic
if "%choice%"=="7" goto get_alerts
if "%choice%"=="8" goto happy_path_100
if "%choice%"=="9" goto happy_path_500
if "%choice%"=="10" goto happy_path_10
if "%choice%"=="11" goto insufficient_funds_high
if "%choice%"=="12" goto insufficient_funds_medium
if "%choice%"=="13" goto check_wallet_after_fail
if "%choice%"=="14" goto concurrent_manual
if "%choice%"=="15" goto concurrent_setup
if "%choice%"=="16" goto concurrent_payment_a
if "%choice%"=="17" goto concurrent_payment_b
if "%choice%"=="18" goto zero_amount
if "%choice%"=="19" goto negative_amount
if "%choice%"=="20" goto invalid_currency
if "%choice%"=="21" goto missing_user_id
if "%choice%"=="22" goto get_payment_by_id
if "%choice%"=="23" goto get_all_payments
if "%choice%"=="24" goto get_wallet_usd
if "%choice%"=="25" goto get_wallet_eur
if "%choice%"=="26" goto get_wallet_invalid
if "%choice%"=="27" goto multiple_small_payments
if "%choice%"=="28" goto rapid_fire_payments
if "%choice%"=="29" goto run_all_happy
if "%choice%"=="30" goto run_all_error
if "%choice%"=="31" goto exit
echo Opcion invalida. Intenta de nuevo.
goto menu

:health
echo.
echo ========================================
echo         HEALTH CHECK
echo ========================================
curl -s http://localhost:8080/health
echo.
pause
goto menu

:ready
echo.
echo ========================================
echo         READINESS CHECK
echo ========================================
curl -s http://localhost:8080/ready
echo.
pause
goto menu

:live
echo.
echo ========================================
echo         LIVENESS CHECK
echo ========================================
curl -s http://localhost:8080/live
echo.
pause
goto menu

:metrics
echo.
echo ========================================
echo         PROMETHEUS METRICS
echo ========================================
curl -s http://localhost:8080/metrics
echo.
pause
goto menu

:demo
echo.
echo ========================================
echo         DEMO PAYMENT FLOW
echo ========================================
curl -s http://localhost:8080/demo
echo.
pause
goto menu

:get_wallet_basic
echo.
echo ========================================
echo         GET WALLET BALANCE
echo ========================================
echo Getting wallet for user: 550e8400-e29b-41d4-a716-446655440001
echo Currency: USD
echo.
curl -s http://localhost:8080/api/v1/wallets/550e8400-e29b-41d4-a716-446655440001/USD
echo.
pause
goto menu

:get_alerts
echo.
echo ========================================
echo         GET ALERTS
echo ========================================
curl -s http://localhost:8080/api/v1/metrics/alerts
echo.
pause
goto menu

:happy_path_100
echo.
echo ========================================
echo    HAPPY PATH - SUCCESSFUL PAYMENT $100
echo ========================================
echo Creating payment for user: 550e8400-e29b-41d4-a716-446655440001
echo Amount: $100.00 USD
echo Expected: SUCCESS (if sufficient funds)
echo.
curl -X POST http://localhost:8080/api/v1/payments ^
  -H "Content-Type: application/json" ^
  -d "{\"user_id\":\"550e8400-e29b-41d4-a716-446655440001\",\"amount\":10000,\"currency\":\"USD\",\"service_id\":\"netflix_subscription\",\"description\":\"Netflix Monthly Subscription - Happy Path Test\"}"
echo.
echo "Guarda el 'id' del pago para consultas posteriores"
pause
goto menu

:happy_path_500
echo.
echo ========================================
echo    HAPPY PATH - LARGE PAYMENT $500
echo ========================================
echo Creating payment for user: 550e8400-e29b-41d4-a716-446655440001
echo Amount: $500.00 USD
echo Expected: SUCCESS (if sufficient funds)
echo.
curl -X POST http://localhost:8080/api/v1/payments ^
  -H "Content-Type: application/json" ^
  -d "{\"user_id\":\"550e8400-e29b-41d4-a716-446655440001\",\"amount\":50000,\"currency\":\"USD\",\"service_id\":\"premium_service\",\"description\":\"Premium Service Payment - Large Amount Test\"}"
echo.
pause
goto menu

:happy_path_10
echo.
echo ========================================
echo    HAPPY PATH - SMALL PAYMENT $10
echo ========================================
echo Creating payment for user: 550e8400-e29b-41d4-a716-446655440001
echo Amount: $10.00 USD
echo Expected: SUCCESS
echo.
curl -X POST http://localhost:8080/api/v1/payments ^
  -H "Content-Type: application/json" ^
  -d "{\"user_id\":\"550e8400-e29b-41d4-a716-446655440001\",\"amount\":1000,\"currency\":\"USD\",\"service_id\":\"micro_transaction\",\"description\":\"Small Payment Test\"}"
echo.
pause
goto menu

:insufficient_funds_high
echo.
echo ========================================
echo  INSUFFICIENT FUNDS - HIGH AMOUNT $10000
echo ========================================
echo Creating payment for user: 550e8400-e29b-41d4-a716-446655440001
echo Amount: $10000.00 USD
echo Expected: FAILED - INSUFFICIENT_FUNDS
echo.
curl -X POST http://localhost:8080/api/v1/payments ^
  -H "Content-Type: application/json" ^
  -d "{\"user_id\":\"550e8400-e29b-41d4-a716-446655440001\",\"amount\":1000000,\"currency\":\"USD\",\"service_id\":\"high_value_service\",\"description\":\"High Value Payment - Should Fail\"}"
echo.
echo "Expected: 400 Bad Request with INSUFFICIENT_FUNDS error"
pause
goto menu

:insufficient_funds_medium
echo.
echo ========================================
echo INSUFFICIENT FUNDS - MEDIUM AMOUNT $5000
echo ========================================
echo Creating payment for user: 550e8400-e29b-41d4-a716-446655440001
echo Amount: $5000.00 USD
echo Expected: FAILED - INSUFFICIENT_FUNDS
echo.
curl -X POST http://localhost:8080/api/v1/payments ^
  -H "Content-Type: application/json" ^
  -d "{\"user_id\":\"550e8400-e29b-41d4-a716-446655440001\",\"amount\":500000,\"currency\":\"USD\",\"service_id\":\"medium_value_service\",\"description\":\"Medium Value Payment - Should Fail\"}"
echo.
pause
goto menu

:check_wallet_after_fail
echo.
echo ========================================
echo    CHECK WALLET AFTER FAILED PAYMENT
echo ========================================
echo Checking wallet balance after failed payment attempts
echo User: 550e8400-e29b-41d4-a716-446655440001
echo Expected: Balance should be unchanged from failed payments
echo.
curl -s http://localhost:8080/api/v1/wallets/550e8400-e29b-41d4-a716-446655440001/USD
echo.
pause
goto menu

:concurrent_manual
echo.
echo ========================================
echo     CONCURRENT PAYMENTS - MANUAL TEST
echo ========================================
echo.
echo INSTRUCCIONES PARA PRUEBA MANUAL:
echo 1. Abrir DOS ventanas de terminal/cmd
echo 2. En ventana 1: Ejecutar opcion 16 (Payment A $800)
echo 3. En ventana 2: Ejecutar opcion 17 (Payment B $600) INMEDIATAMENTE
echo 4. Observar que solo uno debe ser exitoso
echo.
echo Presiona cualquier tecla para continuar...
pause
goto menu

:concurrent_setup
echo.
echo ========================================
echo   CONCURRENT PAYMENTS - SETUP TEST DATA
echo ========================================
echo Verificando saldo actual antes de pruebas concurrentes
echo User: user123 (para pruebas de concurrencia)
echo.
curl -s http://localhost:8080/api/v1/wallets/user123/USD
echo.
echo "Nota: Para pruebas de concurrencia, asegurate de que el saldo sea >= $1000"
pause
goto menu

:concurrent_payment_a
echo.
echo ========================================
echo    CONCURRENT PAYMENT A - $800
echo ========================================
echo Creating payment A for user: user123
echo Amount: $800.00 USD
echo Expected: SUCCESS (if executed first in concurrent scenario)
echo.
curl -X POST http://localhost:8080/api/v1/payments ^
  -H "Content-Type: application/json" ^
  -d "{\"user_id\":\"user123\",\"amount\":80000,\"currency\":\"USD\",\"service_id\":\"concurrent_test_a\",\"description\":\"Concurrent Payment A - Should succeed if first\"}"
echo.
pause
goto menu

:concurrent_payment_b
echo.
echo ========================================
echo    CONCURRENT PAYMENT B - $600
echo ========================================
echo Creating payment B for user: user123
echo Amount: $600.00 USD
echo Expected: FAILED - INSUFFICIENT_FUNDS (if Payment A succeeded first)
echo.
curl -X POST http://localhost:8080/api/v1/payments ^
  -H "Content-Type: application/json" ^
  -d "{\"user_id\":\"user123\",\"amount\":60000,\"currency\":\"USD\",\"service_id\":\"concurrent_test_b\",\"description\":\"Concurrent Payment B - Should fail if A succeeded\"}"
echo.
pause
goto menu

:zero_amount
echo.
echo ========================================
echo        ZERO AMOUNT PAYMENT TEST
echo ========================================
echo Creating payment with amount: $0.00
echo Expected: 400 Bad Request - Invalid amount
echo.
curl -X POST http://localhost:8080/api/v1/payments ^
  -H "Content-Type: application/json" ^
  -d "{\"user_id\":\"550e8400-e29b-41d4-a716-446655440001\",\"amount\":0,\"currency\":\"USD\",\"service_id\":\"zero_test\",\"description\":\"Zero Amount Test\"}"
echo.
pause
goto menu

:negative_amount
echo.
echo ========================================
echo       NEGATIVE AMOUNT PAYMENT TEST
echo ========================================
echo Creating payment with amount: -$50.00
echo Expected: 400 Bad Request - Invalid amount
echo.
curl -X POST http://localhost:8080/api/v1/payments ^
  -H "Content-Type: application/json" ^
  -d "{\"user_id\":\"550e8400-e29b-41d4-a716-446655440001\",\"amount\":-5000,\"currency\":\"USD\",\"service_id\":\"negative_test\",\"description\":\"Negative Amount Test\"}"
echo.
pause
goto menu

:invalid_currency
echo.
echo ========================================
echo       INVALID CURRENCY PAYMENT TEST
echo ========================================
echo Creating payment with currency: XYZ (invalid)
echo Expected: 400 Bad Request - Invalid currency
echo.
curl -X POST http://localhost:8080/api/v1/payments ^
  -H "Content-Type: application/json" ^
  -d "{\"user_id\":\"550e8400-e29b-41d4-a716-446655440001\",\"amount\":5000,\"currency\":\"XYZ\",\"service_id\":\"currency_test\",\"description\":\"Invalid Currency Test\"}"
echo.
pause
goto menu

:missing_user_id
echo.
echo ========================================
echo       MISSING USER ID PAYMENT TEST
echo ========================================
echo Creating payment without user_id
echo Expected: 400 Bad Request - Missing required field
echo.
curl -X POST http://localhost:8080/api/v1/payments ^
  -H "Content-Type: application/json" ^
  -d "{\"amount\":5000,\"currency\":\"USD\",\"service_id\":\"missing_user_test\",\"description\":\"Missing User ID Test\"}"
echo.
pause
goto menu

:get_payment_by_id
echo.
echo ========================================
echo         GET PAYMENT BY ID
echo ========================================
set /p payment_id="Ingresa el Payment ID: "
if "%payment_id%"=="" (
    echo Payment ID no puede estar vacio
    pause
    goto menu
)
curl -s http://localhost:8080/api/v1/payments/%payment_id%
echo.
pause
goto menu

:get_all_payments
echo.
echo ========================================
echo      GET ALL PAYMENTS FOR USER
echo ========================================
echo Getting all payments for user: 550e8400-e29b-41d4-a716-446655440001
echo.
curl -s "http://localhost:8080/api/v1/payments?user_id=550e8400-e29b-41d4-a716-446655440001"
echo.
pause
goto menu

:get_wallet_usd
echo.
echo ========================================
echo         GET WALLET USD
echo ========================================
echo Getting USD wallet for user: 550e8400-e29b-41d4-a716-446655440001
echo.
curl -s http://localhost:8080/api/v1/wallets/550e8400-e29b-41d4-a716-446655440001/USD
echo.
pause
goto menu

:get_wallet_eur
echo.
echo ========================================
echo         GET WALLET EUR
echo ========================================
echo Getting EUR wallet for user: 550e8400-e29b-41d4-a716-446655440001
echo.
curl -s http://localhost:8080/api/v1/wallets/550e8400-e29b-41d4-a716-446655440001/EUR
echo.
pause
goto menu

:get_wallet_invalid
echo.
echo ========================================
echo      GET WALLET INVALID CURRENCY
echo ========================================
echo Getting wallet with invalid currency: XYZ
echo Expected: 400 Bad Request or 404 Not Found
echo.
curl -s http://localhost:8080/api/v1/wallets/550e8400-e29b-41d4-a716-446655440001/XYZ
echo.
pause
goto menu

:multiple_small_payments
echo.
echo ========================================
echo     MULTIPLE SMALL PAYMENTS (5x $20)
echo ========================================
echo Executing 5 small payments of $20 each
echo.
for /L %%i in (1,1,5) do (
    echo Payment %%i of 5...
    curl -X POST http://localhost:8080/api/v1/payments ^
      -H "Content-Type: application/json" ^
      -d "{\"user_id\":\"550e8400-e29b-41d4-a716-446655440001\",\"amount\":2000,\"currency\":\"USD\",\"service_id\":\"multi_test_%%i\",\"description\":\"Multiple Payment Test %%i\"}"
    echo.
    timeout /t 2 /nobreak >nul
)
echo.
echo "Completed 5 payments. Check wallet balance."
pause
goto menu

:rapid_fire_payments
echo.
echo ========================================
echo      RAPID FIRE PAYMENTS (3 QUICK)
echo ========================================
echo Executing 3 rapid payments without delay
echo.
curl -X POST http://localhost:8080/api/v1/payments -H "Content-Type: application/json" -d "{\"user_id\":\"550e8400-e29b-41d4-a716-446655440001\",\"amount\":1500,\"currency\":\"USD\",\"service_id\":\"rapid_1\",\"description\":\"Rapid Payment 1\"}" &
curl -X POST http://localhost:8080/api/v1/payments -H "Content-Type: application/json" -d "{\"user_id\":\"550e8400-e29b-41d4-a716-446655440001\",\"amount\":1500,\"currency\":\"USD\",\"service_id\":\"rapid_2\",\"description\":\"Rapid Payment 2\"}" &
curl -X POST http://localhost:8080/api/v1/payments -H "Content-Type: application/json" -d "{\"user_id\":\"550e8400-e29b-41d4-a716-446655440001\",\"amount\":1500,\"currency\":\"USD\",\"service_id\":\"rapid_3\",\"description\":\"Rapid Payment 3\"}" &
echo.
echo "Executed 3 rapid payments. Check results and wallet balance."
pause
goto menu

:run_all_happy
echo.
echo ========================================
echo        RUN ALL HAPPY PATH TESTS
echo ========================================
echo Executing all happy path scenarios...
echo.
echo "1. Small Payment $10..."
curl -X POST http://localhost:8080/api/v1/payments -H "Content-Type: application/json" -d "{\"user_id\":\"550e8400-e29b-41d4-a716-446655440001\",\"amount\":1000,\"currency\":\"USD\",\"service_id\":\"auto_small\",\"description\":\"Auto Small Payment\"}"
echo.
timeout /t 3 /nobreak >nul

echo "2. Medium Payment $100..."
curl -X POST http://localhost:8080/api/v1/payments -H "Content-Type: application/json" -d "{\"user_id\":\"550e8400-e29b-41d4-a716-446655440001\",\"amount\":10000,\"currency\":\"USD\",\"service_id\":\"auto_medium\",\"description\":\"Auto Medium Payment\"}"
echo.
timeout /t 3 /nobreak >nul

echo "3. Checking final wallet balance..."
curl -s http://localhost:8080/api/v1/wallets/550e8400-e29b-41d4-a716-446655440001/USD
echo.
echo "Happy path tests completed!"
pause
goto menu

:run_all_error
echo.
echo ========================================
echo         RUN ALL ERROR TESTS
echo ========================================
echo Executing all error scenarios...
echo.
echo "1. Zero amount..."
curl -X POST http://localhost:8080/api/v1/payments -H "Content-Type: application/json" -d "{\"user_id\":\"550e8400-e29b-41d4-a716-446655440001\",\"amount\":0,\"currency\":\"USD\",\"service_id\":\"auto_zero\",\"description\":\"Auto Zero Test\"}"
echo.
timeout /t 2 /nobreak >nul

echo "2. Negative amount..."
curl -X POST http://localhost:8080/api/v1/payments -H "Content-Type: application/json" -d "{\"user_id\":\"550e8400-e29b-41d4-a716-446655440001\",\"amount\":-1000,\"currency\":\"USD\",\"service_id\":\"auto_negative\",\"description\":\"Auto Negative Test\"}"
echo.
timeout /t 2 /nobreak >nul

echo "3. Invalid currency..."
curl -X POST http://localhost:8080/api/v1/payments -H "Content-Type: application/json" -d "{\"user_id\":\"550e8400-e29b-41d4-a716-446655440001\",\"amount\":1000,\"currency\":\"XYZ\",\"service_id\":\"auto_currency\",\"description\":\"Auto Currency Test\"}"
echo.
timeout /t 2 /nobreak >nul

echo "4. High amount (insufficient funds)..."
curl -X POST http://localhost:8080/api/v1/payments -H "Content-Type: application/json" -d "{\"user_id\":\"550e8400-e29b-41d4-a716-446655440001\",\"amount\":1000000,\"currency\":\"USD\",\"service_id\":\"auto_high\",\"description\":\"Auto High Amount Test\"}"
echo.

echo "Error tests completed!"
pause
goto menu

:exit
echo.
echo ========================================
echo    GRACIAS POR USAR EL SISTEMA DE PRUEBAS
echo ========================================
echo.
echo Resumen de casos de prueba disponibles:
echo - Health Checks (4 tests)
echo - Happy Path Flow (3 scenarios)
echo - Insufficient Funds Flow (3 scenarios)  
echo - Concurrent Payments Flow (4 tests)
echo - Edge Cases (4 validation tests)
echo - Wallet Operations (3 tests)
echo - Stress Tests (2 scenarios)
echo - Automated Test Suites (2 suites)
echo.
echo Total: 25+ individual test scenarios
echo.
echo Saliendo...
exit /b 0
