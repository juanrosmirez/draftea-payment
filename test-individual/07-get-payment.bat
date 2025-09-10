@echo off
echo ========================================
echo         GET PAYMENT TEST
echo ========================================
echo Testing: GET http://localhost:8081/api/v1/payments/{id}
echo.

set /p payment_id="Ingresa el Payment ID: "
if "%payment_id%"=="" (
    echo ERROR: Payment ID no puede estar vacio
    echo Ejecuta primero 06-create-payment.bat para obtener un ID
    pause
    exit /b 1
)

echo.
echo Consultando pago: %payment_id%
echo.

curl -s http://localhost:8081/api/v1/payments/%payment_id%

echo.
echo ========================================
echo Test completed. Press any key to exit.
pause > nul
