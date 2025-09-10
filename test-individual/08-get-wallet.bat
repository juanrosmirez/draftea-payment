@echo off
echo ========================================
echo         GET WALLET TEST
echo ========================================
echo Testing: GET http://localhost:8081/api/v1/wallets/{user_id}/{currency}
echo User: 550e8400-e29b-41d4-a716-446655440001
echo Currency: USD
echo.

curl -s http://localhost:8081/api/v1/wallets/550e8400-e29b-41d4-a716-446655440001/USD

echo.
echo ========================================
echo Test completed. Press any key to exit.
pause > nul
