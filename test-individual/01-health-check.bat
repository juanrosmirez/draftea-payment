@echo off
echo ========================================
echo         HEALTH CHECK TEST
echo ========================================
echo Testing: http://localhost:8081/health
echo.

curl -s http://localhost:8081/health

echo.
echo ========================================
echo Test completed. Press any key to exit.
pause > nul
