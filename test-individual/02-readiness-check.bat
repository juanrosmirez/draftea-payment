@echo off
echo ========================================
echo        READINESS CHECK TEST
echo ========================================
echo Testing: http://localhost:8081/ready
echo.

curl -s http://localhost:8081/ready

echo.
echo ========================================
echo Test completed. Press any key to exit.
pause > nul
