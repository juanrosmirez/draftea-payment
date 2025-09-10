@echo off
echo ========================================
echo         METRICS TEST
echo ========================================
echo Testing: http://localhost:8081/metrics
echo.

curl -s http://localhost:8081/metrics

echo.
echo ========================================
echo Test completed. Press any key to exit.
pause > nul
