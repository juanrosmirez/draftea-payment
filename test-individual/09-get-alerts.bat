@echo off
echo ========================================
echo         GET ALERTS TEST
echo ========================================
echo Testing: GET http://localhost:8081/api/v1/metrics/alerts
echo.

curl -s http://localhost:8081/api/v1/metrics/alerts

echo.
echo ========================================
echo Test completed. Press any key to exit.
pause > nul
