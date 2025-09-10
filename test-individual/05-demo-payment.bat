@echo off
echo ========================================
echo        DEMO PAYMENT TEST
echo ========================================
echo Testing: http://localhost:8081/demo
echo.

curl -s http://localhost:8081/demo

echo.
echo ========================================
echo Test completed. Press any key to exit.
pause > nul
